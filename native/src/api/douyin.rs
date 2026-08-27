use anyhow::{Context, Result, bail};
use regex::Regex;
use reqwest::Client;
use tracing::{info};
use crate::models::{VideoInfo, Platform};

pub struct DouyinParser { client: Client, cookies: String }

impl DouyinParser {
    pub fn new(_proxy: &str) -> Result<Self> {
        let client = Client::builder().timeout(std::time::Duration::from_secs(15))
            .redirect(reqwest::redirect::Policy::none()).build()?;
        Ok(Self { client, cookies: String::new() })
    }
    pub fn set_cookies(&mut self, c: &str) { self.cookies = c.into(); }

    pub async fn parse(&self, raw_url: &str) -> Result<VideoInfo> {
        let mut url = raw_url.to_string();
        if url.contains("v.douyin.com") {
            if let Ok(r) = self.resolve_short_url(&url).await { url = r; info!("[抖音] 短链→ {}", url); }
        }
        let vid = self.extract_video_id(&url);
        let full = if !vid.is_empty() { format!("https://www.douyin.com/video/{}", vid) } else { url.clone() };
        if !vid.is_empty() { info!("[抖音] 视频ID: {}", vid); }

        // Strategy 1: detail API
        if !vid.is_empty() {
            if let Ok(info) = self.parse_detail_api(&vid).await {
                if !info.video_url.is_empty() { info!("[抖音] 策略1成功"); return Ok(info); }
            }
        }
        // Strategy 2: RENDER_DATA
        if let Ok(info) = self.parse_render_data(&full).await {
            if !info.video_url.is_empty() { info!("[抖音] 策略2成功"); return Ok(info); }
        }
        // Strategy 3: HTML regex
        if let Ok(info) = self.parse_html_regex(&full).await {
            if !info.video_url.is_empty() { info!("[抖音] 策略3成功"); return Ok(info); }
        }
        bail!("所有解析策略失败: {}", raw_url)
    }

    pub async fn resolve_short_url(&self, url: &str) -> Result<String> {
        let resp = self.client.get(url)
            .header("User-Agent", "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 Chrome/126 Mobile Safari/537.36")
            .send().await?;
        if let Some(loc) = resp.headers().get("location") { return Ok(loc.to_str()?.to_string()); }
        bail!("no redirect")
    }

    pub fn extract_video_id(&self, url: &str) -> String {
        for pat in [r"douyin.com/video/(d+)", r"iesdouyin.com/share/video/(d+)", r"modal_id=(d+)"] {
            if let Ok(re) = Regex::new(pat) { if let Some(m) = re.captures(url) { return m[1].to_string(); } }
        }
        String::new()
    }

    pub fn extract_first_url(text: &str) -> Option<String> {
        let re = Regex::new(r"https?://[^\s]+").ok()?;
        re.find(text).map(|m| m.as_str().to_string())
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
        // Get ttwid
        let ttwid = self.get_ttwid().await.unwrap_or_default();
        let url = format!("https://www.douyin.com/aweme/v1/web/aweme/detail/?aweme_id={}&aid=1128&version_name=23.5.0", video_id);
        let mut req = self.client.get(&url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Referer", "https://www.douyin.com/");
        let cookie_str = if !ttwid.is_empty() { format!("ttwid={}", ttwid) } else { "msToken=abcdefg".into() };
        req = req.header("Cookie", &cookie_str);
        let resp = req.send().await?;
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
        } else { (String::new(), String::new()) };
        let cover = self.extract_cover(video);
        let quality_order = ["4k","2k","1080p","720p","480p"];
        let mut selected_url = String::new();
        let mut selected_q = String::new();
        for q in &quality_order { if let Some(u) = urls.get(*q) { if !u.is_empty() { selected_url = u.clone(); selected_q = q.to_string(); break; } } }
        if selected_url.is_empty() { for (q,u) in &urls { if q != "_audio" { selected_url = u.clone(); selected_q = q.clone(); break; } } }
        if selected_url.is_empty() { bail!("no video URL"); }
        let audio_url = urls.get("_audio").cloned().unwrap_or_default();
        let avail: Vec<String> = urls.keys().filter(|k| *k != "_audio").cloned().collect();
        Ok(VideoInfo { title, author, author_id, cover_url: cover, video_url: selected_url, audio_url, selected_quality: selected_q, available_qualities: avail, platform: Platform::Douyin, ..Default::default() })
    }

    fn extract_cover(&self, video: &serde_json::Value) -> String {
        for key in &["cover", "origin_cover"] {
            if let Some(c) = video.get(key) {
                if let Some(list) = c.get("url_list").and_then(|v| v.as_array()) {
                    if let Some(u) = list.first().and_then(|v| v.as_str()) {
                        let url = if u.starts_with("//") { format!("https:{}", u) } else { u.to_string() };
                        return url;
                    }
                }
            }
        }
        String::new()
    }

    fn extract_video_urls(&self, video: &serde_json::Value) -> std::collections::HashMap<String, String> {
        let mut urls = std::collections::HashMap::new();
        if let Some(br) = video.get("bit_rate").and_then(|v| v.as_array()) {
            for item in br {
                let gear = item.get("gear_name").and_then(|v| v.as_str()).unwrap_or("");
                let qt = item.get("quality_type").and_then(|v| v.as_f64()).unwrap_or(0.0);
                let w = item.get("width").and_then(|v| v.as_f64()).unwrap_or(0.0) as i32;
                let h = item.get("height").and_then(|v| v.as_f64()).unwrap_or(0.0) as i32;
                if let Some(u) = item.get("play_addr").and_then(|p| p.get("url_list")).and_then(|l| l.as_array()).and_then(|a| a.first()).and_then(|v| v.as_str()) {
                    let q = map_quality(gear, qt, w, h);
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
        // Try to find detail/video in nested JSON
        if let Some(detail) = find_detail(&data) { return self.extract_from_detail(detail); }
        bail!("video info not in RENDER_DATA")
    }

    async fn parse_html_regex(&self, url: &str) -> Result<VideoInfo> {
        let (body, _cookies) = self.fetch_html(url).await?;
        let re = Regex::new(r#"src="(https?://[^"]*?.mp4[^"]*?)""#)?;
        if let Some(m) = re.captures(&body) {
            return Ok(VideoInfo { video_url: m[1].to_string(), platform: Platform::Douyin, ..Default::default() });
        }
        bail!("no video URL in HTML")
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

fn map_quality(gear: &str, qt: f64, _w: i32, h: i32) -> String {
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
    s
}
