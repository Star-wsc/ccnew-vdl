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
    pub config: AppConfig,
    pub client: Client,
    tasks_file: PathBuf,
}

impl DownloadManager {
    pub async fn new(config: AppConfig) -> Result<Self> {
        let client = Client::builder().timeout(std::time::Duration::from_secs(300)).build()?;
        let tasks_file = PathBuf::from(&config.log_dir).join("tasks.json");
        let _ = std::fs::create_dir_all(&config.download_dir);
        let _ = std::fs::create_dir_all(&config.log_dir);
        let mgr = Self { tasks: Arc::new(RwLock::new(HashMap::new())), config, client, tasks_file };
        mgr.load_tasks().await;
        Ok(mgr)
    }

    pub async fn create_task(&self, url: &str, quality: &str) -> DownloadTask {
        let task = DownloadTask::new(url, quality);
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
        // Parse
        let url = if let Some(u) = extract_first_url(&task.url) { u } else { task.url.clone() };
        let platform = detect_platform(&url);
        let video_info = match platform {
            Platform::Douyin => {
                let mut parser = DouyinParser::new(&self.config.proxy)?;
                if !self.config.douyin_cookie.is_empty() { parser.set_cookies(&self.config.douyin_cookie); }
                parser.parse(&url).await?
            }
            Platform::Bilibili => {
                let parser = BilibiliParser::new(&self.config.proxy)?;
                parser.parse(&url, &task.quality).await?
            }
            _ => anyhow::bail!("不支持的平台"),
        };
        // Update task metadata
        { let mut tasks = self.tasks.write().await;
          if let Some(t) = tasks.get_mut(task_id) {
              t.title = video_info.title.clone(); t.author = video_info.author.clone();
              t.cover_url = video_info.cover_url.clone(); t.video_url = video_info.video_url.clone();
              t.platform = video_info.platform.to_string();
          }
        }
        // Download
        self.update_status(task_id, TaskStatus::Downloading, "").await;
        self.save_tasks().await;
        let output = generate_output_path(&video_info.title, &video_info.platform.to_string(), &self.config.download_dir);
        let has_audio = !video_info.audio_url.is_empty();
        if has_audio {
            let video_tmp = output.with_extension("video.tmp");
            let audio_tmp = output.with_extension("audio.tmp");
            let tasks_ref = self.tasks.clone();
            let tid = task_id.to_string();
            self.download_file(&video_info.video_url, &video_tmp, move |dl, total| {
                let tasks = tasks_ref.clone(); let tid = tid.clone();
                tokio::spawn(async move { update_progress(&tasks, &tid, dl, total).await; });
            }).await?;
            self.download_file(&video_info.audio_url, &audio_tmp, |_, _| {}).await?;
            merge_audio_video(&video_tmp, &audio_tmp, &output).await?;
            let _ = std::fs::remove_file(&video_tmp);
            let _ = std::fs::remove_file(&audio_tmp);
        } else {
            let tasks_ref = self.tasks.clone();
            let tid = task_id.to_string();
            self.download_file(&video_info.video_url, &output, move |dl, total| {
                let tasks = tasks_ref.clone(); let tid = tid.clone();
                tokio::spawn(async move { update_progress(&tasks, &tid, dl, total).await; });
            }).await?;
        }
        { let mut tasks = self.tasks.write().await;
          if let Some(t) = tasks.get_mut(task_id) {
              t.status = TaskStatus::Completed; t.progress = 100; t.file_path = output.to_string_lossy().to_string();
              t.updated_at = chrono::Utc::now().to_rfc3339();
          }
        }
        self.save_tasks().await;
        info!("task {} completed", task_id);
        Ok(())
    }

    pub async fn get_all_tasks(&self) -> Vec<DownloadTask> { self.tasks.read().await.values().cloned().collect() }

    async fn update_status(&self, id: &str, status: TaskStatus, msg: &str) {
        if let Some(t) = self.tasks.write().await.get_mut(id) {
            t.status = status; t.error_message = msg.into(); t.updated_at = chrono::Utc::now().to_rfc3339();
        }
    }

    async fn download_file<F: Fn(u64, u64)>(&self, url: &str, path: &Path, cb: F) -> Result<()> {
        let resp = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Referer", "https://www.douyin.com/").send().await?;
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

    async fn save_tasks(&self) {
        let tasks: Vec<DownloadTask> = self.tasks.read().await.values().cloned().collect();
        if let Ok(data) = serde_json::to_string_pretty(&tasks) { let _ = std::fs::write(&self.tasks_file, data); }
    }

    async fn load_tasks(&self) {
        let Ok(data) = std::fs::read_to_string(&self.tasks_file) else { return; };
        let Ok(mut tasks): std::result::Result<Vec<DownloadTask>, _> = serde_json::from_str(&data) else { return; };
        let mut map = self.tasks.write().await;
        for t in &mut tasks {
            if t.status == TaskStatus::Parsing || t.status == TaskStatus::Downloading {
                t.status = TaskStatus::Failed; t.error_message = "程序重启，任务中断".into();
            }
            map.insert(t.id.clone(), t.clone());
        }
    }
}

async fn merge_audio_video(video: &Path, audio: &Path, output: &Path) -> Result<()> {
    let status = tokio::process::Command::new("ffmpeg")
        .args(["-i", &video.to_string_lossy(), "-i", &audio.to_string_lossy(), "-c:v", "copy", "-c:a", "copy", "-y", &output.to_string_lossy()])
        .status().await?;
    if status.success() { Ok(()) } else { anyhow::bail!("ffmpeg merge failed: {}", status) }
}

async fn update_progress(tasks: &Arc<RwLock<HashMap<String, DownloadTask>>>, id: &str, downloaded: u64, total: u64) {
    if let Some(t) = tasks.write().await.get_mut(id) {
        if total > 0 { t.progress = (downloaded * 100 / total) as i32; }
        t.speed = downloaded as i64; // simplified
        t.updated_at = chrono::Utc::now().to_rfc3339();
    }
}

pub fn detect_platform(url: &str) -> Platform {
    if url.contains("douyin") || url.contains("v.douyin.com") { Platform::Douyin }
    else if url.contains("bilibili") || url.contains("b23.tv") { Platform::Bilibili }
    else { Platform::Unknown }
}

pub fn extract_first_url(text: &str) -> Option<String> {
    let re = regex::Regex::new(r"https?://[^\s]+").ok()?;
    re.find(text).map(|m| m.as_str().to_string())
}

fn sanitize_filename(name: &str) -> String {
    let bad = ['/', '\\', ':', '*', '?', '"', '<', '>', '|'];
    let s: String = name.chars().map(|c| if bad.contains(&c) { '_' } else { c }).collect();
    let s = s.trim().to_string();
    if s.len() > 80 { s[..80].to_string() } else { s }
}

fn generate_output_path(title: &str, platform: &str, dir: &str) -> PathBuf {
    let prefix = match platform { "douyin" => "dy_", "bilibili" => "bz_", _ => "vd_" };
    let name = format!("{}{}", prefix, sanitize_filename(title));
    let mut path = PathBuf::from(dir).join(format!("{}.mp4", name));
    let mut i = 1;
    while path.exists() {
        path = PathBuf::from(dir).join(format!("{}({}).mp4", name, i));
        i += 1;
    }
    path
}
