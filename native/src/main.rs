mod ui;
mod api;
mod download;
mod config;
mod ffmpeg;

use config::AppConfig;
use tracing::info;

slint::include_modules!();

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_max_level(tracing::Level::INFO)
        .init();

    let config = AppConfig::load()?;
    info!("CCNEW Native v{} 启动", env!("CARGO_PKG_VERSION"));
    info!("下载目录: {}", config.download_dir.display());

    let app = AppWindow::new()?;
    
    // 解析回调
    let app_weak = app.as_weak();
    app.on_parse_clicked(move |url| {
        let url = url.to_string();
        if url.is_empty() { return; }
        let Some(app) = app_weak.upgrade() else { return };
        app.set_status_text("解析中…".into());
        // TODO: 异步解析逻辑
    });
    
    // 回车提交
    let app_weak = app.as_weak();
    app.on_url_submitted(move |url| {
        let url = url.to_string();
        if url.is_empty() { return; }
        let Some(app) = app_weak.upgrade() else { return };
        app.set_status_text("解析中…".into());
        // TODO: 异步解析逻辑
    });

    app.run()?;
    Ok(())
}