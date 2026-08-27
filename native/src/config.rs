use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AppConfig {
    pub download_dir: String,
    pub log_dir: String,
    pub bilibili_cookie: String,
    pub douyin_cookie: String,
    pub proxy: String,
    #[serde(default = "default_quality")]
    pub quality: String,
}

fn default_quality() -> String { "1080p".to_string() }

impl Default for AppConfig {
    fn default() -> Self {
        let dl = dirs::download_dir().unwrap_or_else(|| PathBuf::from("."))
            .to_string_lossy().to_string();
        let log = dirs::data_local_dir().unwrap_or_else(|| PathBuf::from("."))
            .join("CCNEW-VideoDownloader-Native").join("logs")
            .to_string_lossy().to_string();
        Self {
            download_dir: dl,
            log_dir: log,
            bilibili_cookie: String::new(),
            douyin_cookie: String::new(),
            proxy: String::new(),
            quality: "1080p".to_string(),
        }
    }
}

impl AppConfig {
    pub fn load() -> anyhow::Result<Self> {
        let p = Self::config_path();
        let mut cfg = if p.exists() {
            serde_json::from_str::<AppConfig>(&std::fs::read_to_string(&p)?).unwrap_or_default()
        } else {
            Self::default()
        };
        // 修复旧配置中缺失的字段
        let defaults = Self::default();
        if cfg.download_dir.is_empty() { cfg.download_dir = defaults.download_dir; }
        if cfg.log_dir.is_empty() { cfg.log_dir = defaults.log_dir; }
        if cfg.quality.is_empty() { cfg.quality = defaults.quality; }
        cfg.save()?;
        Ok(cfg)
    }
    pub fn save(&self) -> anyhow::Result<()> {
        let p = Self::config_path();
        if let Some(d) = p.parent() { std::fs::create_dir_all(d)?; }
        std::fs::write(&p, serde_json::to_string_pretty(self)?)?;
        Ok(())
    }
    fn config_path() -> PathBuf {
        dirs::data_local_dir().unwrap_or_else(|| PathBuf::from("."))
            .join("CCNEW-VideoDownloader-Native").join("config.json")
    }
}
