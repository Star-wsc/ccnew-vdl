use ccnew_native::download::DownloadManager;
use ccnew_native::config::AppConfig;
use ccnew_native::models::TaskStatus;
use std::sync::Arc;
use tracing::info;

slint::include_modules!();

fn fmt_speed(bps: u64) -> String {
    if bps == 0 { return String::new(); }
    let units = ["B/s","KB/s","MB/s","GB/s"];
    let (mut v, mut i) = (bps as f64, 0);
    while v >= 1024.0 && i < 3 { v /= 1024.0; i += 1; }
    if v >= 100.0 { format!("{:.0} {}", v, units[i]) } else { format!("{:.1} {}", v, units[i]) }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt().with_max_level(tracing::Level::INFO).init();
    let config = AppConfig::load()?;
    info!("CCNEW Native v{} 启动", env!("CARGO_PKG_VERSION"));
    let mgr = Arc::new(DownloadManager::new(config).await?);
    let app = AppWindow::new()?;

    app.on_parse_clicked({ let m = mgr.clone(); move |url| {
        let url = url.to_string(); if url.is_empty() { return; }
        let m = m.clone();
        tokio::spawn(async move { let t = m.create_task(&url, "auto").await; m.execute_task(&t.id).await.ok(); });
    }});

    app.on_url_submitted({ let m = mgr.clone(); move |url| {
        let url = url.to_string(); if url.is_empty() { return; }
        let m = m.clone();
        tokio::spawn(async move { let t = m.create_task(&url, "auto").await; m.execute_task(&t.id).await.ok(); });
    }});

    let mp = mgr.clone();
    let weak = app.as_weak();
    let timer = slint::Timer::default();
    timer.start(slint::TimerMode::Repeated, std::time::Duration::from_secs(2), move || {
        let Some(app) = weak.upgrade() else { return };
        let rt = tokio::runtime::Handle::current();
        let tasks = rt.block_on(mp.get_all_tasks());
        let (mut dl, mut done, mut fail) = (0i32, 0i32, 0i32);
        let items: Vec<TaskData> = tasks.iter().map(|t| {
            match t.status { TaskStatus::Downloading => dl+=1, TaskStatus::Completed => done+=1, TaskStatus::Failed => fail+=1, _=>{} }
            TaskData { id: t.id.clone().into(), title: t.title.clone().into(), author: t.author.clone().into(),
                platform: t.platform.clone().into(),
                status: match &t.status { TaskStatus::Pending=>"pending",TaskStatus::Parsing=>"parsing",TaskStatus::Downloading=>"downloading",TaskStatus::Completed=>"completed",TaskStatus::Failed=>"failed" }.into(),
                speed: fmt_speed(t.speed as u64).into(), progress: t.progress as f32 }
        }).collect();
        app.set_tasks(std::rc::Rc::new(slint::VecModel::from(items)).into());
        app.set_downloading_count(dl); app.set_completed_count(done); app.set_failed_count(fail);
        app.set_status_text(format!("就绪 · {} 个任务", tasks.len()).into());
    });

    app.run()?; Ok(())
}
