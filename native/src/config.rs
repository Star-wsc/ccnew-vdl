use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct AppConfig {
    pub download_dir: String, pub log_dir: String,
    pub bilibili_cookie: String, pub douyin_cookie: String, pub proxy: String,
}
impl AppConfig {
    pub fn load() -> anyhow::Result<Self> {
        let p = Self::config_path();
        if p.exists() { Ok(serde_json::from_str(&std::fs::read_to_string(&p)?).unwrap_or_default()) }
        else { let c = Self::default(); c.save()?; Ok(c) }
    }
    pub fn save(&self) -> anyhow::Result<()> {
        let p = Self::config_path();
        if let Some(d) = p.parent() { std::fs::create_dir_all(d)?; }
        std::fs::write(&p, serde_json::to_string_pretty(self)?)?; Ok(())
    }
    fn config_path() -> PathBuf {
        dirs::data_local_dir().unwrap_or_else(|| PathBuf::from(".")).join("CCNEW-VideoDownloader-Native").join("config.json")
    }
}
