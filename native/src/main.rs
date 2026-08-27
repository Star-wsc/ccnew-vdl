#![cfg_attr(windows, windows_subsystem = "windows")]

use ccnew_native::config::AppConfig;
use ccnew_native::download::DownloadManager;
use ccnew_native::server::{AppState, build_router};
use std::sync::Arc;
use tracing::info;

fn main() -> anyhow::Result<()> {
    // Tokio runtime
    let rt = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?;
    let _guard = rt.enter();

    // 日志
    let log_dir = dirs::data_local_dir()
        .unwrap_or_else(|| std::path::PathBuf::from("."))
        .join("CCNEW-VideoDownloader-Native")
        .join("logs");
    let _ = std::fs::create_dir_all(&log_dir);
    let log_file = std::fs::OpenOptions::new()
        .create(true).append(true)
        .open(log_dir.join("app.log")).ok();
    if let Some(f) = log_file {
        tracing_subscriber::fmt()
            .with_max_level(tracing::Level::INFO)
            .with_writer(std::sync::Mutex::new(f))
            .init();
    } else {
        tracing_subscriber::fmt()
            .with_max_level(tracing::Level::INFO)
            .init();
    }

    let config = rt.block_on(async { AppConfig::load() })?;
    info!("CCNEW Native v{} 启动", env!("CARGO_PKG_VERSION"));

    let mgr = Arc::new(rt.block_on(DownloadManager::new(config))?);

    // 内嵌 HTTP 服务器 - 绑定随机端口
    let state = AppState {
        mgr: mgr.clone(),
        logs: Arc::new(tokio::sync::RwLock::new(Vec::new())),
        collections: Arc::new(tokio::sync::RwLock::new(std::collections::HashMap::new())),
    };
    let app = build_router(state);
    let port = find_free_port()?;
    let addr = std::net::SocketAddr::from(([127, 0, 0, 1], port));
    info!("HTTP 服务器启动于 http://127.0.0.1:{}", port);

    rt.spawn(async move {
        let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
        axum::serve(listener, app).await.unwrap();
    });

    // 等待服务器就绪
    std::thread::sleep(std::time::Duration::from_millis(200));

    // 创建原生窗口 + WebView
    let url = format!("http://127.0.0.1:{}", port);
    create_window(&url)?;

    Ok(())
}

fn find_free_port() -> anyhow::Result<u16> {
    let listener = std::net::TcpListener::bind("127.0.0.1:0")?;
    Ok(listener.local_addr()?.port())
}

fn create_window(url: &str) -> anyhow::Result<()> {
    use tao::event::{Event, StartCause, WindowEvent};
    use tao::event_loop::{ControlFlow, EventLoopBuilder};
    use tao::window::WindowBuilder;
    use wry::WebViewBuilder;

    let event_loop = EventLoopBuilder::new().build();
    let window = WindowBuilder::new()
        .with_title("CCNEW 视频下载器 · Native v1.3.4")
        .with_inner_size(tao::dpi::LogicalSize::new(1100.0, 720.0))
        .with_min_inner_size(tao::dpi::LogicalSize::new(800.0, 500.0))
        .build(&event_loop)?;

    let _webview = WebViewBuilder::new()
        .with_url(url)
        .build(&window)?;

    event_loop.run(move |event, _, control_flow| {
        *control_flow = ControlFlow::Wait;
        match event {
            Event::WindowEvent { event: WindowEvent::CloseRequested, .. } => {
                *control_flow = ControlFlow::Exit;
            }
            _ => {}
        }
    });
}
