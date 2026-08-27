use anyhow::{Context, Result, bail};
use regex::Regex;
use reqwest::Client;
use tracing::info;
use crate::models::{VideoInfo, Platform};

pub struct DouyinParser {
    client: Client,
    cookies: String,
}

impl DouyinParser {
    pub fn new(proxy: &str) -> Result<Self> {
        let mut builder = Client::builder()
            .timeout(std::time::Duration::from_secs(15))
            .redirect(reqwest::redirect::Policy::none());
        if !proxy.is_empty() {
            builder = builder.proxy(reqwest::Proxy::all(proxy)?);
        }
        Ok(Self { client: builder.build()?, cookies: String::new() })
    }

    pub fn set_cookies(&mut self, c: &str) { self.cookies = c.into(); }

    pub async fn parse(&self, raw_url: &str) -> Result<VideoInfo> {
        let mut url = raw_url.to_string();
        info!("[抖音] 开始解析: {}", raw_url);

        // 解析短链接
        if url.contains("v.douyin.com") {
            if let Ok(r) = self.resolve_short_url(&url).await {
                url = r;
                info!("[抖音] 短链→ {}", url);
            }
        }

        let vid = self.extract_video_id(&url);
        let full = if !vid.is_empty() {
            info!("[抖音] 视频ID: {}", vid);
            format!("https://www.douyin.com/video/{}", vid)
        } else {
            url.clone()
        };

        // 策略1: iesdouyin分享页 + 移动端UA (最有效)
        if !vid.is_empty() {
            let share_url = format!("https://www.iesdouyin.com/share/video/{}/", vid);
            info!("[抖音] 策略1: iesdouyin分享页+移动端UA");
            if let Ok(info) = self.parse_with_mobile_ua(&share_url).await {
                if !info.video_url.is_empty() {
                    let mut info = info;
                    self.enrich_audio_url(&mut info, &vid).await;
                    info!("[抖音] 策略1成功: title={}", info.title);
                    return Ok(info);
                }
            }
        }

        // 策略2: douyin.com + 移动端UA
        info!("[抖音] 策略2: douyin.com+移动端UA");
        if let Ok(info) = self.parse_with_mobile_ua(&full).await {
            if !info.video_url.is_empty() {
                let mut info = info;
                if !vid.is_empty() { self.enrich_audio_url(&mut info, &vid).await; }
                info!("[抖音] 策略2成功");
                return Ok(info);
            }
        }

        // 策略3: 桌面UA Detail API
        if !vid.is_empty() {
            info!("[抖音] 策略3: Detail API");
            if let Ok(info) = self.parse_detail_api(&vid).await {
                if !info.video_url.is_empty() {
                    let mut info = info;
                    self.enrich_audio_url(&mut info, &vid).await;
                    info!("[抖音] 策略3成功");
                    return Ok(info);
                }
            }
        }

        // 策略4: RENDER_DATA
        info!("[抖音] 策略4: RENDER_DATA");
        if let Ok(info) = self.parse_render_data(&full).await {
            if !info.video_url.is_empty() {
                info!("[抖音] 策略4成功");
                return Ok(info);
            }
        }

        // 策略5: 多UA尝试
        info!("[抖音] 策略5: 多UA尝试");
        if !vid.is_empty() {
            let share_url = format!("https://www.iesdouyin.com/share/video/{}/", vid);
            if let Ok(info) = self.parse_with_alternate_uas(&share_url).await {
                if !info.video_url.is_empty() {
                    let mut info = info;
                    self.enrich_audio_url(&mut info, &vid).await;
                    info!("[抖音] 策略5成功");
                    return Ok(info);
                }
            }
        }

        // 策略6: 第三方API
        info!("[抖音] 策略6: 第三方API");
        if let Ok(info) = self.parse_via_api(&full).await {
            if !info.video_url.is_empty() {
                info!("[抖音] 策略6成功");
                return Ok(info);
            }
        }

        bail!("所有解析策略失败: {}", raw_url)
    }

    // 移动端UA解析
    async fn parse_with_mobile_ua(&self, url: &str) -> Result<VideoInfo> {
        let ua = "Mozilla/5.0 (Linux; Android 10; SM-G981B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36";
        self.parse_with_ua(url, ua).await
    }

    // 多UA尝试
    async fn parse_with_alternate_uas(&self, url: &str) -> Result<VideoInfo> {
        let uas = [
            "com.ss.android.ugc.aweme/330201 (Linux; U; Android 13; zh_CN; SM-G991B; Build/TP1A.220624.014)",
            "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X; zh_CN; Scale/3.00)",
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15",
        ];
        for ua in &uas {
            if let Ok(info) = self.parse_with_ua(url, ua).await {
                if !info.video_url.is_empty() { return Ok(info); }
            }
        }
        bail!("所有备用UA策略失败")
    }

    // 通用UA解析 (从HTML提取视频信息)
    async fn parse_with_ua(&self, url: &str, ua: &str) -> Result<VideoInfo> {
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(30))
            .redirect(reqwest::redirect::Policy::default())
            .build()?;
        let resp = client.get(url)
            .header("User-Agent", ua)
            .header("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
            .header("Accept-Language", "zh-CN,zh;q=0.9")
            .send().await?;
        let html = resp.text().await?;
        self.extract_from_html(&html)
    }

    // 从HTML提取视频信息
    fn extract_from_html(&self, html: &str) -> Result<VideoInfo> {
        let mut video_urls = std::collections::HashMap::new();
        let mut download_urls = std::collections::HashMap::new();

        // 提取download_addr
        let patterns = vec![
            (r#""download_addr"[^}]*"url_list"\s*:\s*\["([^"]+)"#, "download"),
            (r#""download"[^}]*"url_list"\s*:\s*\["([^"]+)"#, "download"),
            (r#""play_addr"[^}]*"url_list"\s*:\s*\["([^"]+)"#, "play"),
        ];

        for (pattern, url_type) in &patterns {
            if let Ok(re) = Regex::new(pattern) {
                if let Some(m) = re.captures(html) {
                    let u = process_url(&m[1]);
                    if *url_type == "download" {
                        download_urls.insert("1080p".to_string(), u);
                    } else {
                        video_urls.insert("1080p".to_string(), u);
                    }
                }
            }
        }

        // 提取bit_rate中的不同清晰度
        if let Ok(re) = Regex::new(r#""bit_rate"\s*:\s*\[([^\]]+)\]"#) {
            if let Some(m) = re.captures(html) {
                let bitrates = &m[1];
                let quality_map = vec![
                    ("4k", vec!["4k", "2160p", "uhd"]),
                    ("2k", vec!["2k", "1440p", "qhd"]),
                    ("1080p", vec!["1080p", "fhd", "full_hd"]),
                    ("720p", vec!["720p", "hd", "high"]),
                    ("480p", vec!["480p", "sd", "normal"]),
                ];
                for (q, keywords) in &quality_map {
                    for kw in keywords {
                        let pattern = format!(r#"(?i)"gear_name"\s*:\s*"[^"]*{}[^"]*"[^}}]*"play_addr"[^}}]*"url_list"\s*:\s*\["([^"]+)"#, kw);
                        if let Ok(re) = Regex::new(&pattern) {
                            if let Some(m) = re.captures(bitrates) {
                                video_urls.insert(q.to_string(), process_url(&m[1]));
                                break;
                            }
                        }
                    }
                }
            }
        }

        // 合并所有URL
        let mut all_urls = video_urls;
        for (k, v) in download_urls { all_urls.insert(k, v); }

        if all_urls.is_empty() {
            anyhow::bail!("HTML中未找到视频URL");
        }

        // 选择最高清晰度
        let priority = ["4k", "2k", "1080p", "720p", "480p"];
        let mut selected_url = String::new();
        let mut selected_quality = String::new();
        for q in &priority {
            if let Some(u) = all_urls.get(*q) {
                selected_url = u.clone();
                selected_quality = q.to_string();
                break;
            }
        }
        if selected_url.is_empty() {
            for (q, u) in &all_urls {
                selected_url = u.clone();
                selected_quality = q.clone();
                break;
            }
        }

        // 提取标题、作者、封面
        let title = Regex::new(r#""desc"\s*:\s*"([^"]*)""#)
            .ok().and_then(|re| re.captures(html))
            .map(|m| m[1].to_string()).unwrap_or_default();
        let author = Regex::new(r#""nickname"\s*:\s*"([^"]*)""#)
            .ok().and_then(|re| re.captures(html))
            .map(|m| m[1].to_string()).unwrap_or("未知作者".to_string());
        let cover_url = Regex::new(r#""cover"[^}]*"url_list"\s*:\s*\["([^"]+)"#)
            .ok().and_then(|re| re.captures(html))
            .map(|m| process_url(&m[1])).unwrap_or_default();

        info!("[抖音-HTML] 选择清晰度: {}", selected_quality);

        Ok(VideoInfo {
            title, author, cover_url,
            video_url: selected_url,
            platform: Platform::Douyin,
            ..Default::default()
        })
    }

    // 第三方API
    async fn parse_via_api(&self, url: &str) -> Result<VideoInfo> {
        let apis = [
            format!("https://api.douyin.wtf/api?url={}", urlencoding::encode(url)),
            format!("https://www.douyin.wtf/api?url={}", urlencoding::encode(url)),
        ];
        for api_url in &apis {
            if let Ok(info) = self.fetch_from_api(api_url).await {
                if !info.video_url.is_empty() { return Ok(info); }
            }
        }
        bail!("第三方API均失败")
    }

    async fn fetch_from_api(&self, api_url: &str) -> Result<VideoInfo> {
        let client = Client::builder().timeout(std::time::Duration::from_secs(15)).build()?;
        let resp = client.get(api_url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
            .send().await?;
        let json: serde_json::Value = resp.json().await?;
        let code = json.get("code").and_then(|v| v.as_i64()).unwrap_or(-1);
        let data = json.get("data").context("no data")?;
        let video_url = data.get("video_url").and_then(|v| v.as_str()).unwrap_or("");
        if code != 0 || video_url.is_empty() { anyhow::bail!("API error"); }
        Ok(VideoInfo {
            title: data.get("title").and_then(|v| v.as_str()).unwrap_or("").into(),
            author: data.get("author").and_then(|v| v.as_str()).unwrap_or("").into(),
            cover_url: data.get("cover_url").and_then(|v| v.as_str()).unwrap_or("").into(),
            video_url: video_url.into(),
            platform: Platform::Douyin,
            ..Default::default()
        })
    }

    // ====== 原有策略保留 ======

    pub async fn resolve_short_url(&self, url: &str) -> Result<String> {
        let resp = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 Chrome/126 Mobile Safari/537.36")
            .header("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
            .send().await?;
        if let Some(loc) = resp.headers().get("location") { return Ok(loc.to_str()?.to_string()); }
        bail!("no redirect")
    }

    pub fn extract_video_id(&self, url: &str) -> String {
        for pat in [r"douyin\.com/video/(\d+)", r"iesdouyin\.com/share/video/(\d+)", r"modal_id=(\d+)"] {
            if let Ok(re) = Regex::new(pat) {
                if let Some(m) = re.captures(url) { return m[1].to_string(); }
            }
        }
        String::new()
    }

    async fn fetch_html(&self, url: &str) -> Result<(String, std::collections::HashMap<String, String>)> {
        let mut req = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Accept-Language", "zh-CN,zh;q=0.9")
            .header("Referer", "https://www.douyin.com/");
        if !self.cookies.is_empty() { req = req.header("Cookie", &self.cookies); }
        else { req = req.header("Cookie", "msToken=abcdefg"); }
        let resp = req.send().await?;
        let mut cookies = std::collections::HashMap::new();
        for c in resp.cookies() { cookies.insert(c.name().to_string(), c.value().to_string()); }
        let body = resp.text().await?;
        Ok((body, cookies))
    }

    async fn parse_detail_api(&self, video_id: &str) -> Result<VideoInfo> {
        let ttwid = self.get_ttwid().await.unwrap_or_default();
        let url = format!("https://www.douyin.com/aweme/v1/web/aweme/detail/?aweme_id={}&aid=1128&version_name=23.5.0", video_id);
        let cookie_str = if !ttwid.is_empty() { format!("ttwid={}", ttwid) } else { "msToken=abcdefg".into() };
        let resp = self.client.get(&url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Referer", "https://www.douyin.com/")
            .header("Cookie", &cookie_str)
            .send().await?;
        let json: serde_json::Value = resp.json().await?;
        let detail = json.get("aweme_detail").context("no aweme_detail")?;
        self.extract_from_detail(detail)
    }

    fn extract_from_detail(&self, detail: &serde_json::Value) -> Result<VideoInfo> {
        let video = detail.get("video").context("no video")?;
        let urls = self.extract_video_urls(video);
        let title = detail.get("desc").and_then(|v| v.as_str()).unwrap_or("").to_string();
        let (author, author_id) = if let Some(a) = detail.get("author") {
            (a.get("nickname").and_then(|v| v.as_str()).unwrap_or("").to_string(),
             a.get("unique_id").and_then(|v| v.as_str()).or_else(|| a.get("short_id").and_then(|v| v.as_str())).unwrap_or("").to_string())
        } else { ("未知作者".into(), String::new()) };
        let cover_url = video.get("cover").and_then(|c| c.get("url_list")).and_then(|l| l.as_array()).and_then(|a| a.first()).and_then(|v| v.as_str()).map(process_url).unwrap_or_default();
        let mut video_urls = urls;
        let audio_url = video_urls.remove("_audio").unwrap_or_default();
        let priority = ["4k", "2k", "1080p", "720p", "480p"];
        let mut selected_url = String::new();
        let mut selected_quality = String::new();
        for q in &priority {
            if let Some(u) = video_urls.get(*q) {
                selected_url = u.clone(); selected_quality = q.to_string(); break;
            }
        }
        if selected_url.is_empty() {
            for (q, u) in &video_urls { selected_url = u.clone(); selected_quality = q.clone(); break; }
        }
        if selected_url.is_empty() { anyhow::bail!("未找到视频URL"); }
        Ok(VideoInfo {
            title, author, author_id, cover_url,
            video_url: selected_url, audio_url,
            platform: Platform::Douyin,
            ..Default::default()
        })
    }

    fn extract_video_urls(&self, video: &serde_json::Value) -> std::collections::HashMap<String, String> {
        let mut urls = std::collections::HashMap::new();
        // play_addr
        if let Some(u) = video.get("play_addr").and_then(|p| p.get("url_list")).and_then(|l| l.as_array()).and_then(|a| a.first()).and_then(|v| v.as_str()) {
            urls.insert("1080p".into(), process_url(u));
        }
        // bit_rate
        if let Some(br) = video.get("bit_rate").and_then(|v| v.as_array()) {
            for item in br {
                let gear = item.get("gear_name").and_then(|v| v.as_str()).unwrap_or("");
                let qt = item.get("quality_type").and_then(|v| v.as_f64()).unwrap_or(0.0);
                let h = item.get("height").and_then(|v| v.as_f64()).unwrap_or(0.0) as i32;
                if let Some(u) = item.get("play_addr").and_then(|p| p.get("url_list")).and_then(|l| l.as_array()).and_then(|a| a.first()).and_then(|v| v.as_str()) {
                    let q = map_quality(gear, qt, h);
                    if !q.is_empty() { urls.insert(q, process_url(u)); }
                }
            }
        }
        // Audio
        if let Some(bra) = video.get("bit_rate_audio").and_then(|v| v.as_array()) {
            for item in bra {
                if let Some(meta) = item.get("audio_meta") {
                    if let Some(url_list) = meta.get("url_list") {
                        for key in &["main_url", "backup_url", "fallback_url"] {
                            if let Some(u) = url_list.get(*key).and_then(|v| v.as_str()) {
                                if !u.is_empty() {
                                    let u = if u.starts_with("//") { format!("https:{}", u) } else { u.to_string() };
                                    urls.insert("_audio".into(), u); break;
                                }
                            }
                        }
                    }
                }
                if urls.contains_key("_audio") { break; }
            }
        }
        urls
    }

    async fn parse_render_data(&self, url: &str) -> Result<VideoInfo> {
        let (body, _cookies) = self.fetch_html(url).await?;
        let re = Regex::new(r#"<script id="RENDER_DATA" type="application/json">([^<]+)</script>"#)?;
        let caps = re.captures(&body).context("RENDER_DATA not found")?;
        let decoded = urlencoding::decode(&caps[1])?;
        let data: serde_json::Value = serde_json::from_str(&decoded)?;
        if let Some(detail) = find_detail(&data) { return self.extract_from_detail(detail); }
        bail!("video info not in RENDER_DATA")
    }

    async fn parse_html_regex(&self, url: &str) -> Result<VideoInfo> {
        let (body, _cookies) = self.fetch_html(url).await?;
        self.extract_from_html(&body)
    }

    async fn get_ttwid(&self) -> Result<String> {
        let resp = self.client.post("https://ttwid.bytedance.com/ttwid/union/register/")
            .header("Content-Type", "application/json")
            .body(r#"{"region":"cn","aid":1768,"needFid":false,"service":"www.ixigua.com","migrate_priority":0,"cbUrlProtocol":"https","union":true}"#)
            .send().await?;
        for c in resp.cookies() { if c.name() == "ttwid" { return Ok(c.value().to_string()); } }
        bail!("no ttwid")
    }

    pub async fn enrich_audio_url(&self, info: &mut VideoInfo, video_id: &str) {
        if !info.audio_url.is_empty() { return; }
        if let Ok(detail_info) = self.parse_detail_api(video_id).await {
            if !detail_info.audio_url.is_empty() {
                info.audio_url = detail_info.audio_url;
                info!("[抖音-enrich] 已补充音频URL");
            }
        }
    }
}

fn find_detail<'a>(v: &'a serde_json::Value) -> Option<&'a serde_json::Value> {
    if let Some(d) = v.get("aweme").and_then(|a| a.get("detail")) { return Some(d); }
    if let Some(d) = v.get("app").and_then(|a| a.get("videoInfo")) { return Some(d); }
    if let Some(obj) = v.as_object() {
        for val in obj.values() {
            if let Some(d) = val.get("detail") { return Some(d); }
            if let Some(d) = val.get("videoInfo") { return Some(d); }
        }
    }
    None
}

fn map_quality(gear: &str, qt: f64, h: i32) -> String {
    if h >= 2160 || gear.contains("4k") || gear.contains("uhd_4k") { return "4k".into(); }
    if h >= 1440 || gear.contains("2k") || gear.contains("uhd_2k") { return "2k".into(); }
    if h >= 1080 || gear.contains("hd") || qt == 2.0 { return "1080p".into(); }
    if h >= 720 || gear.contains("normal") || qt == 3.0 { return "720p".into(); }
    if h >= 480 || gear.contains("sp_hd") || qt == 4.0 { return "480p".into(); }
    if !gear.is_empty() { return gear.to_string(); }
    String::new()
}

fn process_url(u: &str) -> String {
    let mut s = u.replace("playwm", "play");
    if let Some(i) = s.find("?ratio=") { s.truncate(i); }
    // 移除水印参数
    if let Ok(re) = Regex::new(r"[?&]watermark=\d+") { s = re.replace_all(&s, "").to_string(); }
    if let Ok(re) = Regex::new(r"[?&]ratio=\w+") { s = re.replace_all(&s, "").to_string(); }
    if s.starts_with("//") { s = format!("https:{}", s); }
    s
}
