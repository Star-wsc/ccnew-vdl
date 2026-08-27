use anyhow::{Result, Context};
use reqwest::Client;
use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{info, error};
use crate::api::douyin::DouyinParser;
use crate::api::bilibili::BilibiliParser;
use crate::config::AppConfig;
use crate::models::{DownloadTask, TaskStatus, Platform};

pub struct DownloadManager {
    pub tasks: Arc<RwLock<HashMap<String, DownloadTask>>>,
    pub config: Arc<RwLock<AppConfig>>,
    pub client: Client,
    tasks_file: PathBuf,
}

impl DownloadManager {
    pub async fn new(config: AppConfig) -> Result<Self> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(600))
            .build()?;
        let tasks_file = PathBuf::from(&config.log_dir).join("tasks.json");
        let _ = std::fs::create_dir_all(&config.download_dir);
        let _ = std::fs::create_dir_all(&config.log_dir);
        let mgr = Self {
            tasks: Arc::new(RwLock::new(HashMap::new())),
            config: Arc::new(RwLock::new(config)),
            client,
            tasks_file,
        };
        mgr.load_tasks().await;
        Ok(mgr)
    }

    pub async fn create_task(&self, url: &str, quality: &str) -> DownloadTask {
        let q = if quality == "auto" {
            self.config.read().await.quality.clone()
        } else {
            quality.to_string()
        };
        let task = DownloadTask::new(url, &q);
        let id = task.id.clone();
        self.tasks.write().await.insert(id, task.clone());
        self.save_tasks().await;
        task
    }

    pub async fn execute_task(&self, task_id: &str) -> Result<()> {
        let result = self.execute_inner(task_id).await;
        if let Err(ref e) = result {
            error!("task {} failed: {:#}", task_id, e);
            self.update_status(task_id, TaskStatus::Failed, &format!("{:#}", e)).await;
            self.save_tasks().await;
        }
        result
    }

    async fn execute_inner(&self, task_id: &str) -> Result<()> {
        let task = self.tasks.read().await.get(task_id).cloned().context("task not found")?;
        self.update_status(task_id, TaskStatus::Parsing, "").await;

        let url = if let Some(u) = extract_first_url(&task.url) { u } else { task.url.clone() };
        let platform = detect_platform(&url);
        let cfg = self.config.read().await;

        let video_info = match platform {
            Platform::Douyin => {
                let mut parser = DouyinParser::new(&cfg.proxy)?;
                if !cfg.douyin_cookie.is_empty() { parser.set_cookies(&cfg.douyin_cookie); }
                parser.parse(&url).await?
            }
            Platform::Bilibili => {
                let mut parser = BilibiliParser::new(&cfg.proxy)?;
                if !cfg.bilibili_cookie.is_empty() { parser.set_cookies(&cfg.bilibili_cookie); }
                parser.parse(&url, &task.quality).await?
            }
            _ => anyhow::bail!("不支持的平台"),
        };
        drop(cfg);

        // 更新任务元数据
        {
            let mut tasks = self.tasks.write().await;
            if let Some(t) = tasks.get_mut(task_id) {
                t.title = video_info.title.clone();
                t.author = video_info.author.clone();
                t.cover_url = video_info.cover_url.clone();
                t.video_url = video_info.video_url.clone();
                t.platform = video_info.platform.to_string();
            }
        }

        self.update_status(task_id, TaskStatus::Downloading, "").await;
        self.save_tasks().await;

        let cfg = self.config.read().await;
        let download_dir = cfg.download_dir.clone();
        let douyin_cookie = cfg.douyin_cookie.clone();
        let bilibili_cookie = cfg.bilibili_cookie.clone();
        drop(cfg);

        let output = generate_output_path(&video_info.title, &video_info.platform.to_string(), &download_dir);
        let has_audio = !video_info.audio_url.is_empty();

        match video_info.platform {
            Platform::Douyin => {
                if has_audio {
                    info!("[抖音] DASH音视频分离，下载并合并...");
                    let video_tmp = output.with_extension("video.tmp");
                    let audio_tmp = output.with_extension("audio.tmp");
                    let cookies = parse_cookies(&douyin_cookie);
                    let tasks_ref = self.tasks.clone();
                    let tid = task_id.to_string();
                    self.download_douyin(&video_info.video_url, &video_tmp, &cookies, move |dl, _| {
                        let tasks = tasks_ref.clone(); let tid = tid.clone();
                        tokio::spawn(async move {
                            if let Some(t) = tasks.write().await.get_mut(&tid) { t.downloaded_bytes = dl; }
                        });
                    }).await?;
                    self.download_douyin(&video_info.audio_url, &audio_tmp, &cookies, |_, _| {}).await?;
                    merge_audio_video(&video_tmp, &audio_tmp, &output).await?;
                    let _ = std::fs::remove_file(&video_tmp);
                    let _ = std::fs::remove_file(&audio_tmp);
                    info!("[抖音] 音视频合并成功");
                } else {
                    let cookies = parse_cookies(&douyin_cookie);
                    let tasks_ref = self.tasks.clone();
                    let tid = task_id.to_string();
                    self.download_douyin(&video_info.video_url, &output, &cookies, move |dl, _| {
                        let tasks = tasks_ref.clone(); let tid = tid.clone();
                        tokio::spawn(async move {
                            if let Some(t) = tasks.write().await.get_mut(&tid) { t.downloaded_bytes = dl; }
                        });
                    }).await?;
                }
            }
            Platform::Bilibili => {
                if has_audio {
                    info!("[B站] DASH格式，下载并合并...");
                    let video_tmp = output.with_extension("video.m4s");
                    let audio_tmp = output.with_extension("audio.m4s");
                    let video_clean = output.with_extension("video.clean.mp4");
                    let audio_clean = output.with_extension("audio.clean.m4a");

                    // 带重试下载
                    self.download_with_retry(&video_info.video_url, &video_tmp, &bilibili_cookie, 3).await?;
                    self.download_with_retry(&video_info.audio_url, &audio_tmp, &bilibili_cookie, 3).await?;

                    // 清理M4S头部
                    remove_m4s_header(&video_tmp, &video_clean)?;
                    remove_m4s_header(&audio_tmp, &audio_clean)?;

                    // 合并
                    merge_audio_video(&video_clean, &audio_clean, &output).await?;

                    // 清理临时文件
                    let _ = std::fs::remove_file(&video_tmp);
                    let _ = std::fs::remove_file(&audio_tmp);
                    let _ = std::fs::remove_file(&video_clean);
                    let _ = std::fs::remove_file(&audio_clean);
                    info!("[B站] 音视频合并成功");
                } else {
                    let tasks_ref = self.tasks.clone();
                    let tid = task_id.to_string();
                    self.download_bilibili(&video_info.video_url, &output, &bilibili_cookie, move |dl, _| {
                        let tasks = tasks_ref.clone(); let tid = tid.clone();
                        tokio::spawn(async move {
                            if let Some(t) = tasks.write().await.get_mut(&tid) { t.downloaded_bytes = dl; }
                        });
                    }).await?;
                }
            }
            _ => anyhow::bail!("不支持的平台"),
        }

        {
            let mut tasks = self.tasks.write().await;
            if let Some(t) = tasks.get_mut(task_id) {
                t.status = TaskStatus::Completed;
                t.progress = 100;
                t.file_path = output.to_string_lossy().to_string();
                t.updated_at = chrono::Utc::now().to_rfc3339();
            }
        }
        self.save_tasks().await;
        info!("task {} completed: {}", task_id, output.display());
        Ok(())
    }

    pub async fn get_all_tasks(&self) -> Vec<DownloadTask> {
        self.tasks.read().await.values().cloned().collect()
    }

    // ====== 删除/选择方法 ======

    pub async fn delete_task(&self, task_id: &str) {
        self.tasks.write().await.remove(task_id);
        self.save_tasks().await;
    }

    pub async fn toggle_selection(&self, index: usize) {
        let mut tasks = self.tasks.write().await;
        let ids: Vec<String> = tasks.keys().cloned().collect();
        if let Some(id) = ids.get(index) {
            if let Some(t) = tasks.get_mut(id) { t.selected = !t.selected; }
        }
    }

    pub async fn select_all(&self) {
        let mut tasks = self.tasks.write().await;
        let all_selected = tasks.values().all(|t| t.selected);
        for t in tasks.values_mut() { t.selected = !all_selected; }
    }

    pub async fn delete_selected(&self) {
        self.tasks.write().await.retain(|_, t| !t.selected);
        self.save_tasks().await;
    }

    pub async fn clear_all(&self) {
        self.tasks.write().await.clear();
        self.save_tasks().await;
    }

    // ====== 抖音下载 (带Cookie和Content-Type检查) ======

    async fn download_douyin<F: Fn(u64, u64)>(&self, url: &str, path: &Path, cookies: &HashMap<String, String>, cb: F) -> Result<()> {
        let mut req = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
            .header("Referer", "https://www.douyin.com/")
            .header("Accept", "*/*")
            .header("Accept-Encoding", "identity")
            .header("Connection", "keep-alive");
        for (k, v) in cookies { req = req.header("Cookie", format!("{}={}", k, v)); }

        let resp = req.send().await?;
        let status = resp.status().as_u16();
        if status != 200 && status != 206 {
            anyhow::bail!("HTTP {}", status);
        }

        // Content-Type检查
        let ct = resp.headers().get("content-type").and_then(|v| v.to_str().ok()).unwrap_or("");
        if !ct.is_empty() && !ct.contains("video") && !ct.contains("mp4") && !ct.contains("octet-stream") {
            // 可能返回了HTML错误页
            let bytes = resp.bytes().await?;
            if bytes.len() > 10 {
                let head = std::str::from_utf8(&bytes[..std::cmp::min(100, bytes.len())]).unwrap_or("");
                if head.starts_with("<!DOCTYPE") || head.starts_with("<html") {
                    anyhow::bail!("服务器返回HTML而非视频流");
                }
            }
            // 不是HTML，写入文件
            tokio::fs::write(path, &bytes).await?;
            cb(bytes.len() as u64, bytes.len() as u64);
            return Ok(());
        }

        let total = resp.content_length().unwrap_or(0);
        let mut stream = resp.bytes_stream();
        let mut file = tokio::fs::File::create(path).await?;
        use futures_util::StreamExt;
        use tokio::io::AsyncWriteExt;
        let mut downloaded: u64 = 0;
        while let Some(chunk) = stream.next().await {
            let chunk = chunk?;
            file.write_all(&chunk).await?;
            downloaded += chunk.len() as u64;
            cb(downloaded, total);
        }
        Ok(())
    }

    // ====== B站下载 (带重试和正确Headers) ======

    async fn download_bilibili<F: Fn(u64, u64)>(&self, url: &str, path: &Path, cookies: &str, cb: F) -> Result<()> {
        for attempt in 0..3 {
            if attempt > 0 {
                info!("[B站] 重试第{}次...", attempt);
                tokio::time::sleep(std::time::Duration::from_secs(attempt as u64 * 2)).await;
            }
            match self.download_bilibili_once(url, path, cookies, &cb).await {
                Ok(()) => return Ok(()),
                Err(e) => {
                    if attempt == 2 { return Err(e); }
                    error!("[B站] 下载失败: {:#}", e);
                }
            }
        }
        anyhow::bail!("B站下载失败(重试3次)")
    }

    async fn download_bilibili_once<F: Fn(u64, u64)>(&self, url: &str, path: &Path, cookies: &str, cb: &F) -> Result<()> {
        let mut req = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
            .header("Referer", "https://www.bilibili.com")
            .header("Accept", "*/*")
            .header("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
            .header("Origin", "https://www.bilibili.com")
            .header("Sec-Fetch-Dest", "video")
            .header("Sec-Fetch-Mode", "no-cors")
            .header("Sec-Fetch-Site", "cross-site");
        if !cookies.is_empty() { req = req.header("Cookie", cookies); }

        let resp = req.send().await?;
        let status = resp.status().as_u16();
        if status != 200 {
            anyhow::bail!("HTTP {}", status);
        }

        // Content-Type检查
        let ct = resp.headers().get("content-type").and_then(|v| v.to_str().ok()).unwrap_or("");
        if !ct.is_empty() && ct != "application/octet-stream" && ct != "video/mp4" && ct != "audio/mp4" {
            let bytes = resp.bytes().await?;
            if bytes.len() > 10 {
                let head = std::str::from_utf8(&bytes[..std::cmp::min(100, bytes.len())]).unwrap_or("");
                if head.starts_with("<!DOCTYPE") || head.starts_with("<html") {
                    anyhow::bail!("服务器返回HTML而非视频流");
                }
            }
            tokio::fs::write(path, &bytes).await?;
            cb(bytes.len() as u64, bytes.len() as u64);
            return Ok(());
        }

        let total = resp.content_length().unwrap_or(0);
        let mut stream = resp.bytes_stream();
        let mut file = tokio::fs::File::create(path).await?;
        use futures_util::StreamExt;
        use tokio::io::AsyncWriteExt;
        let mut downloaded: u64 = 0;
        while let Some(chunk) = stream.next().await {
            let chunk = chunk?;
            file.write_all(&chunk).await?;
            downloaded += chunk.len() as u64;
            cb(downloaded, total);
        }
        Ok(())
    }

    // 带重试的通用下载
    async fn download_with_retry(&self, url: &str, path: &Path, cookies: &str, max_retries: u32) -> Result<()> {
        for attempt in 0..max_retries {
            if attempt > 0 {
                info!("重试第{}次...", attempt);
                tokio::time::sleep(std::time::Duration::from_secs(attempt as u64 * 2)).await;
            }
            match self.download_bilibili_once(url, path, cookies, &|_, _| {}).await {
                Ok(()) => return Ok(()),
                Err(e) => {
                    if attempt == max_retries - 1 { return Err(e); }
                    error!("下载失败: {:#}", e);
                }
            }
        }
        anyhow::bail!("下载失败(重试{}次)", max_retries)
    }

    // ====== 内部方法 ======

    async fn update_status(&self, id: &str, status: TaskStatus, msg: &str) {
        if let Some(t) = self.tasks.write().await.get_mut(id) {
            t.status = status;
            t.error_message = msg.into();
            t.updated_at = chrono::Utc::now().to_rfc3339();
        }
    }

    async fn save_tasks(&self) {
        let tasks: Vec<DownloadTask> = self.tasks.read().await.values().cloned().collect();
        if let Ok(data) = serde_json::to_string_pretty(&tasks) {
            let _ = std::fs::write(&self.tasks_file, data);
        }
    }

    async fn load_tasks(&self) {
        let Ok(data) = std::fs::read_to_string(&self.tasks_file) else { return; };
        let Ok(mut tasks): std::result::Result<Vec<DownloadTask>, _> = serde_json::from_str(&data) else { return; };
        let mut map = self.tasks.write().await;
        for t in &mut tasks {
            if t.status == TaskStatus::Parsing || t.status == TaskStatus::Downloading {
                t.status = TaskStatus::Failed;
                t.error_message = "程序重启，任务中断".into();
            }
            t.selected = false;
            map.insert(t.id.clone(), t.clone());
        }
    }
}

// ====== M4S头部清理 ======
fn remove_m4s_header(input: &Path, output: &Path) -> Result<()> {
    let data = std::fs::read(input)?;
    if data.len() < 8 {
        std::fs::write(output, &data)?;
        return Ok(());
    }
    // 检查前8字节是否全为0
    let all_zero = data[..8].iter().all(|&b| b == 0);
    if all_zero {
        std::fs::write(output, &data[8..])?;
    } else {
        std::fs::write(output, &data)?;
    }
    Ok(())
}

// ====== FFmpeg合并 ======
async fn merge_audio_video(video: &Path, audio: &Path, output: &Path) -> Result<()> {
    let status = tokio::process::Command::new("ffmpeg")
        .args(["-i", &video.to_string_lossy(), "-i", &audio.to_string_lossy(),
               "-c:v", "copy", "-c:a", "copy", "-y", &output.to_string_lossy()])
        .creation_flags(0x08000000) // CREATE_NO_WINDOW
        .status().await?;
    if status.success() { Ok(()) } else { anyhow::bail!("ffmpeg merge failed: {}", status) }
}

// ====== Cookie解析 ======
fn parse_cookies(cookie_str: &str) -> HashMap<String, String> {
    let mut map = HashMap::new();
    for part in cookie_str.split(';') {
        let part = part.trim();
        if let Some((k, v)) = part.split_once('=') {
            map.insert(k.trim().to_string(), v.trim().to_string());
        }
    }
    map
}

// ====== 工具函数 ======
pub fn detect_platform(url: &str) -> Platform {
    if url.contains("douyin") || url.contains("v.douyin.com") || url.contains("iesdouyin") { Platform::Douyin }
    else if url.contains("bilibili") || url.contains("b23.tv") { Platform::Bilibili }
    else { Platform::Unknown }
}

pub fn extract_first_url(text: &str) -> Option<String> {
    let re = regex::Regex::new(r"https?://[^\s]+").ok()?;
    re.find(text).map(|m| m.as_str().to_string())
}

fn sanitize_filename(name: &str) -> String {
    let bad = ['/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r'];
    let s: String = name.chars().map(|c| if bad.contains(&c) { '_' } else { c }).collect();
    let s = s.trim().to_string();
    if s.len() > 80 { s[..80].to_string() } else { s }
}

fn generate_output_path(title: &str, platform: &str, dir: &str) -> PathBuf {
    let prefix = match platform { "douyin" => "dy_", "bilibili" => "bz_", _ => "vd_" };
    let name = format!("{}{}", prefix, sanitize_filename(title));
    if name == format!("{}_", prefix) {
        let fallback = format!("{}video_{}", prefix, chrono::Utc::now().timestamp());
        return PathBuf::from(dir).join(format!("{}.mp4", fallback));
    }
    let mut path = PathBuf::from(dir).join(format!("{}.mp4", name));
    let mut i = 1;
    while path.exists() {
        path = PathBuf::from(dir).join(format!("{}({}).mp4", name, i));
        i += 1;
    }
    path
}
