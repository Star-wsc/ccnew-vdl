use anyhow::{Result, bail, Context};
use regex::Regex;
use reqwest::Client;
use tracing::info;
use crate::models::{VideoInfo, Platform};

pub struct BilibiliParser { client: Client, cookies: String }

impl BilibiliParser {
    pub fn new(_proxy: &str) -> Result<Self> {
        Ok(Self {
            client: Client::builder()
                .timeout(std::time::Duration::from_secs(30))
                .build()?,
            cookies: String::new(),
        })
    }
    pub fn set_cookies(&mut self, c: &str) { self.cookies = c.into(); }

    pub async fn parse(&self, url: &str, quality: &str) -> Result<VideoInfo> {
        let bvid = extract_bvid(url).or_else(|| extract_aid(url).map(|a| format!("av{}", a)));
        let bvid = bvid.context("无法提取B站视频ID")?;
        info!("[B站] 解析: {}", bvid);

        let info = self.fetch_video_info(&bvid).await?;
        let cid = info.cid;
        let (video_url, audio_url, selected_quality) = self.fetch_play_url(&bvid, cid, quality).await?;

        Ok(VideoInfo {
            title: info.title,
            author: info.author,
            cover_url: info.cover_url,
            video_url,
            audio_url,
            selected_quality,
            platform: Platform::Bilibili,
            ..Default::default()
        })
    }

    async fn fetch_video_info(&self, bvid: &str) -> Result<BiliInfo> {
        let url = format!("https://api.bilibili.com/x/web-interface/view?bvid={}", bvid);
        let mut req = self.client.get(&url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Referer", "https://www.bilibili.com/");
        if !self.cookies.is_empty() { req = req.header("Cookie", &self.cookies); }
        let json: serde_json::Value = req.send().await?.json().await?;
        let data = json.get("data").context("B站API无data")?;
        let cid = data.get("cid").and_then(|v| v.as_i64()).unwrap_or(0);
        let title = data.get("title").and_then(|v| v.as_str()).unwrap_or("").to_string();
        let author = data.get("owner").and_then(|o| o.get("name")).and_then(|v| v.as_str()).unwrap_or("").to_string();
        let cover = data.get("pic").and_then(|v| v.as_str()).unwrap_or("").to_string();
        // 修复URL前缀
        let cover = if cover.starts_with("//") { format!("https:{}", cover) } else { cover };
        Ok(BiliInfo { title, author, cover_url: cover, cid })
    }

    async fn fetch_play_url(&self, bvid: &str, cid: i64, quality: &str) -> Result<(String, String, String)> {
        let qn = quality_to_qn(quality);
        let url = format!("https://api.bilibili.com/x/player/playurl?bvid={}&cid={}&qn={}&fnval=4048&fourk=1", bvid, cid, qn);
        let mut req = self.client.get(&url)
            .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
            .header("Referer", "https://www.bilibili.com/");
        if !self.cookies.is_empty() { req = req.header("Cookie", &self.cookies); }
        let json: serde_json::Value = req.send().await?.json().await?;
        let data = json.get("data").context("B站playurl无data")?;

        if let Some(dash) = data.get("dash") {
            // DASH格式: 分离的视频+音频流
            let video_url = dash.get("video")
                .and_then(|v| v.as_array())
                .and_then(|a| a.first())
                .and_then(|v| v.get("baseUrl").or(v.get("base_url")))
                .and_then(|v| v.as_str())
                .unwrap_or("").to_string();
            let audio_url = dash.get("audio")
                .and_then(|v| v.as_array())
                .and_then(|a| a.first())
                .and_then(|v| v.get("baseUrl").or(v.get("base_url")))
                .and_then(|v| v.as_str())
                .unwrap_or("").to_string();
            let actual_qn = data.get("quality").and_then(|v| v.as_i64()).unwrap_or(qn as i64);
            info!("[B站] DASH格式, 视频={}, 音频={}", !video_url.is_empty(), !audio_url.is_empty());
            return Ok((video_url, audio_url, qn_to_name(actual_qn as i32)));
        }

        if let Some(durl) = data.get("durl").and_then(|v| v.as_array()) {
            if let Some(first) = durl.first() {
                let url = first.get("url").and_then(|v| v.as_str()).unwrap_or("").to_string();
                info!("[B站] DURL格式");
                return Ok((url, String::new(), qn_to_name(qn)));
            }
        }

        bail!("无播放地址")
    }
}

struct BiliInfo { title: String, author: String, cover_url: String, cid: i64 }

fn extract_bvid(url: &str) -> Option<String> {
    let re = Regex::new(r"BV[a-zA-Z0-9]+").ok()?;
    re.find(url).map(|m| m.as_str().to_string())
}

fn extract_aid(url: &str) -> Option<String> {
    let re = Regex::new(r"(?:av|avid=)(\d+)").ok()?;
    re.captures(url).map(|c| c[1].to_string())
}

fn quality_to_qn(q: &str) -> i32 {
    match q {
        "4k" => 127, "1080p60" => 116, "1080p+" => 112, "1080p" => 80,
        "720p" => 64, "480p" => 32, "360p" => 16, _ => 80,
    }
}

fn qn_to_name(qn: i32) -> String {
    match qn {
        127 | 120 => "4k", 116 => "1080p60", 112 => "1080p+", 80 => "1080p",
        64 => "720p", 32 => "480p", 16 => "360p", _ => "auto",
    }.into()
}
