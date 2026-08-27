use std::path::PathBuf;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppConfig {
    pub download_dir: PathBuf,
    pub port: u16,
    pub bilibili_cookie: String,
    pub douyin_cookie: String,
    pub ffmpeg_path: PathBuf,
    pub log_dir: PathBuf,
}

impl Default for AppConfig {
    fn default() -> Self {
        let base = dirs::download_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join("CCNEW-VideoDownloader");
        Self {
            download_dir: base,
            port: 18001, // 与 Go 版 18000 不冲突
            bilibili_cookie: String::new(),
            douyin_cookie: String::new(),
            ffmpeg_path: PathBuf::from("ffmpeg"),
            log_dir: dirs::data_local_dir()
                .unwrap_or_else(|| PathBuf::from("."))
                .join("CCNEW-VideoDownloader")
                .join("logs"),
        }
    }
}

impl AppConfig {
    pub fn load() -> anyhow::Result<Self> {
        let config_path = Self::config_path();
        if config_path.exists() {
            let data = std::fs::read_to_string(&config_path)?;
            Ok(serde_json::from_str(&data)?)
        } else {
            let config = Self::default();
            config.save()?;
            Ok(config)
        }
    }

    pub fn save(&self) -> anyhow::Result<()> {
        let config_path = Self::config_path();
        if let Some(parent) = config_path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        std::fs::write(&config_path, serde_json::to_string_pretty(self)?)?;
        Ok(())
    }

    fn config_path() -> PathBuf {
        dirs::data_local_dir()
            .unwrap_or_else(|| PathBuf::from("."))
            .join("CCNEW-VideoDownloader")
            .join("config.json")
    }
}