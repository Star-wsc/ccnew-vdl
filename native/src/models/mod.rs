use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum Platform {
    #[serde(rename = "douyin")] Douyin,
    #[serde(rename = "bilibili")] Bilibili,
    #[serde(rename = "unknown")] Unknown,
}
impl Default for Platform { fn default() -> Self { Self::Unknown } }
impl std::fmt::Display for Platform {
    fn fmt(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
        match self { Self::Douyin => write!(f,"douyin"), Self::Bilibili => write!(f,"bilibili"), _ => write!(f,"unknown") }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum TaskStatus {
    #[serde(rename = "pending")] Pending,
    #[serde(rename = "parsing")] Parsing,
    #[serde(rename = "downloading")] Downloading,
    #[serde(rename = "completed")] Completed,
    #[serde(rename = "failed")] Failed,
}
impl Default for TaskStatus { fn default() -> Self { Self::Pending } }

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct DownloadTask {
    pub id: String, pub url: String, pub title: String, pub author: String,
    pub cover_url: String, pub video_url: String, pub quality: String, pub platform: String,
    pub status: TaskStatus, pub progress: i32, pub speed: i64,
    pub file_path: String, pub file_size: i64, pub error_message: String,
    pub created_at: String, pub updated_at: String,
    #[serde(default)] pub downloaded_bytes: u64,
    #[serde(default)] pub selected: bool,
}
impl DownloadTask {
    pub fn new(url: &str, quality: &str) -> Self {
        let now = chrono::Utc::now().to_rfc3339();
        Self { id: uuid::Uuid::new_v4().to_string(), url: url.into(), quality: quality.into(), created_at: now.clone(), updated_at: now, ..Default::default() }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct VideoInfo {
    pub title: String, pub author: String, pub author_id: String, pub cover_url: String,
    pub video_url: String, pub audio_url: String, pub download_url: String,
    pub selected_quality: String, pub available_qualities: Vec<String>, pub platform: Platform,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CollectionVideoInfo {
    pub bvid: String, pub aid: i64, pub cid: i64, pub video_id: String,
    pub url: String, pub title: String, pub author: String, pub cover_url: String,
    pub duration: i32, pub page: i32,
}
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CreateTaskRequest { pub url: String, pub quality: String }
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreateCollectionRequest {
    pub url: String, pub title: String, pub videos: Vec<CollectionVideoInfo>,
    pub quality: String, pub auto_download: bool, pub selected_indices: Vec<usize>,
}
