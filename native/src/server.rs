use axum::{
    Router,
    extract::{Path as AxumPath, Query, State},
    http::StatusCode,
    response::{IntoResponse, Response, Html},
    routing::{get, post, delete},
    Json,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tokio::sync::RwLock;
use std::collections::HashMap;
use crate::download::DownloadManager;
use crate::config::AppConfig;

#[derive(Clone)]
pub struct AppState {
    pub mgr: Arc<DownloadManager>,
    pub logs: Arc<RwLock<Vec<LogEntry>>>,
    pub collections: Arc<RwLock<HashMap<String, CollectionInfo>>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEntry {
    pub timestamp: String,
    pub level: String,
    pub task_id: String,
    pub message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CollectionInfo {
    pub id: String,
    pub url: String,
    pub title: String,
    pub author: String,
    pub cover_url: String,
    pub total_count: i32,
    pub videos: Vec<CollectionVideo>,
    pub status: String,
    pub quality: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CollectionVideo {
    pub title: String,
    pub author: String,
    pub cover_url: String,
    pub url: String,
    pub status: String,
    pub progress: i32,
    pub speed: i64,
    pub file_path: String,
    pub error_message: String,
}

#[derive(Deserialize)]
pub struct CreateTaskReq {
    pub url: String,
    pub quality: Option<String>,
}

#[derive(Deserialize)]
pub struct SettingsReq {
    pub douyin_cookie: Option<String>,
    pub bilibili_cookie: Option<String>,
    pub proxy: Option<String>,
}

#[derive(Deserialize)]
pub struct PathReq {
    pub path: String,
}

#[derive(Deserialize)]
pub struct CollectionCreateReq {
    pub url: String,
    pub title: Option<String>,
    pub videos: Option<Vec<serde_json::Value>>,
    pub quality: Option<String>,
    pub auto_download: Option<bool>,
    pub selected_indices: Option<Vec<usize>>,
}

pub fn build_router(state: AppState) -> Router {
    Router::new()
        // 静态文件
        .route("/", get(index))
        .route("/static/index.html", get(classic_index))
        .route("/static/index-v2.html", get(index))
        // 任务API
        .route("/api/tasks", get(list_tasks))
        .route("/api/tasks", post(create_task))
        .route("/api/tasks/{id}/execute", post(execute_task))
        .route("/api/tasks/{id}/retry", post(retry_task))
        .route("/api/tasks/{id}", delete(delete_task))
        .route("/api/tasks/{id}/download", get(download_file))
        .route("/api/tasks/{id}/status", get(task_status))
        // 统计
        .route("/api/stats", get(stats))
        // 配置
        .route("/api/config", get(get_config))
        .route("/api/settings", post(update_settings))
        .route("/api/download-dir", post(set_download_dir))
        .route("/api/browse-folder", post(browse_folder))
        // Cookie
        .route("/api/bilibili/cookie", get(get_bilibili_cookie))
        .route("/api/bilibili/cookie", post(set_bilibili_cookie))
        // 合集
        .route("/api/collections", get(list_collections))
        .route("/api/collections", post(create_collection))
        .route("/api/collections/{id}", get(get_collection))
        .route("/api/collections/{id}", delete(delete_collection))
        .route("/api/collections/{id}/download", post(download_collection))
        // 日志
        .route("/api/logs", get(get_logs))
        .route("/api/logs", delete(clear_logs))
        // 图片代理
        .route("/api/proxy/image", get(proxy_image))
        // 控制台(兼容)
        .route("/api/console/visible", get(console_visible))
        .route("/api/console/toggle", post(console_toggle))
        // 从预览创建任务
        .route("/api/tasks/create-from-preview", post(create_from_preview))
        // 合集订阅
        .route("/api/collections/{id}/subscribe", post(subscribe_collection))
        // 合集内单视频操作
        .route("/api/collections/{id}/videos/{idx}/download", post(download_collection_video))
        .route("/api/collections/{id}/videos/{idx}/file", delete(delete_collection_video_file))
        // B站登录
        .route("/api/bilibili/login", post(bilibili_login))
        // 更新
        .route("/api/update", post(check_update))
        .with_state(state)
}

// ====== 静态文件 ======
async fn index() -> Html<&'static str> {
    Html(include_str!("../../static/index-v2.html"))
}

async fn classic_index() -> Html<&'static str> {
    Html(include_str!("../../static/index.html"))
}

// ====== 任务 ======
async fn list_tasks(State(state): State<AppState>) -> Json<serde_json::Value> {
    let tasks = state.mgr.get_all_tasks().await;
    Json(serde_json::json!(tasks))
}

async fn create_task(
    State(state): State<AppState>,
    Json(req): Json<CreateTaskReq>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    let quality = req.quality.as_deref().unwrap_or("auto");
    // preview模式: 只解析不下载
    if quality == "preview" {
        let url = if let Some(u) = crate::download::extract_first_url(&req.url) { u } else { req.url.clone() };
        let platform = crate::download::detect_platform(&url);
        let cfg = state.mgr.config.read().await;
        let info = match platform {
            crate::models::Platform::Douyin => {
                let mut parser = crate::api::douyin::DouyinParser::new(&cfg.proxy).map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
                if !cfg.douyin_cookie.is_empty() { parser.set_cookies(&cfg.douyin_cookie); }
                parser.parse(&url).await.map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?
            }
            crate::models::Platform::Bilibili => {
                let mut parser = crate::api::bilibili::BilibiliParser::new("").map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;
                if !cfg.bilibili_cookie.is_empty() { parser.set_cookies(&cfg.bilibili_cookie); }
                parser.parse(&url, "1080p").await.map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?
            }
            _ => return Err((StatusCode::BAD_REQUEST, "不支持的平台".into())),
        };
        return Ok(Json(serde_json::json!({
            "title": info.title,
            "author": info.author,
            "cover_url": info.cover_url,
            "video_url": info.video_url,
            "platform": info.platform.to_string(),
            "audio_url": info.audio_url,
            "selected_quality": info.selected_quality,
            "available_qualities": info.available_qualities,
        })));
    }
    let task = state.mgr.create_task(&req.url, quality).await;
    let id = task.id.clone();
    let mgr = state.mgr.clone();
    tokio::spawn(async move { mgr.execute_task(&id).await.ok(); });
    Ok(Json(serde_json::json!(task)))
}

async fn execute_task(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    let mgr = state.mgr.clone();
    tokio::spawn(async move { mgr.execute_task(&id).await.ok(); });
    Ok(Json(serde_json::json!({"status": "ok"})))
}

async fn retry_task(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    {
        let mut tasks = state.mgr.tasks.write().await;
        if let Some(t) = tasks.get_mut(&id) {
            if t.status == crate::models::TaskStatus::Failed {
                t.status = crate::models::TaskStatus::Pending;
                t.error_message = String::new();
            }
        }
    }
    let mgr = state.mgr.clone();
    tokio::spawn(async move { mgr.execute_task(&id).await.ok(); });
    Ok(Json(serde_json::json!({"status": "ok"})))
}

async fn delete_task(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
    Query(params): Query<HashMap<String, String>>,
) -> Json<serde_json::Value> {
    // 删除本地文件
    if params.get("deleteFile").map(|s| s.as_str()) == Some("true") {
        let tasks = state.mgr.tasks.read().await;
        if let Some(t) = tasks.get(&id) {
            if !t.file_path.is_empty() {
                let _ = std::fs::remove_file(&t.file_path);
            }
        }
    }
    state.mgr.delete_task(&id).await;
    Json(serde_json::json!({"status": "ok"}))
}

async fn download_file(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Response {
    let tasks = state.mgr.tasks.read().await;
    if let Some(t) = tasks.get(&id) {
        if !t.file_path.is_empty() && std::path::Path::new(&t.file_path).exists() {
            let filename = std::path::Path::new(&t.file_path)
                .file_name().unwrap_or_default().to_string_lossy().to_string();
            let data = match std::fs::read(&t.file_path) {
                Ok(d) => d,
                Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
            };
            return (
                StatusCode::OK,
                [
                    ("Content-Type", "video/mp4".to_string()),
                    ("Content-Disposition", format!("attachment; filename=\"{}\"", filename)),
                    ("Content-Length", data.len().to_string()),
                ],
                data,
            ).into_response();
        }
    }
    (StatusCode::NOT_FOUND, "文件不存在").into_response()
}

async fn task_status(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Json<serde_json::Value> {
    let tasks = state.mgr.tasks.read().await;
    if let Some(t) = tasks.get(&id) {
        Json(serde_json::json!({
            "status": format!("{:?}", t.status).to_lowercase(),
            "progress": t.progress,
            "speed": t.speed,
            "error_message": t.error_message,
            "file_path": t.file_path,
        }))
    } else {
        Json(serde_json::json!({"status": "not_found"}))
    }
}

// ====== 统计 ======
async fn stats(State(state): State<AppState>) -> Json<serde_json::Value> {
    let tasks = state.mgr.get_all_tasks().await;
    let total = tasks.len();
    let downloading = tasks.iter().filter(|t| t.status == crate::models::TaskStatus::Downloading).count();
    let completed = tasks.iter().filter(|t| t.status == crate::models::TaskStatus::Completed).count();
    let failed = tasks.iter().filter(|t| t.status == crate::models::TaskStatus::Failed).count();
    let pending = tasks.iter().filter(|t| t.status == crate::models::TaskStatus::Pending).count();
    Json(serde_json::json!({
        "total": total, "downloading": downloading, "completed": completed,
        "failed": failed, "pending": pending,
    }))
}

// ====== 配置 ======
async fn get_config(State(state): State<AppState>) -> Json<serde_json::Value> {
    let cfg = state.mgr.config.read().await;
    Json(serde_json::json!({
        "download_dir": cfg.download_dir,
        "version": env!("CARGO_PKG_VERSION"),
        "first_run": false,
        "total_tasks": state.mgr.tasks.read().await.len(),
        "platform": "windows",
    }))
}

async fn update_settings(
    State(state): State<AppState>,
    Json(req): Json<SettingsReq>,
) -> Json<serde_json::Value> {
    let mut cfg = state.mgr.config.write().await;
    if let Some(c) = req.douyin_cookie { cfg.douyin_cookie = c; }
    if let Some(c) = req.bilibili_cookie { cfg.bilibili_cookie = c; }
    if let Some(p) = req.proxy { cfg.proxy = p; }
    let _ = cfg.save();
    Json(serde_json::json!({"status": "ok"}))
}

async fn set_download_dir(
    State(state): State<AppState>,
    Json(req): Json<PathReq>,
) -> Json<serde_json::Value> {
    let mut cfg = state.mgr.config.write().await;
    cfg.download_dir = req.path;
    let _ = cfg.save();
    let _ = std::fs::create_dir_all(&cfg.download_dir);
    Json(serde_json::json!({"status": "ok"}))
}

async fn browse_folder() -> Json<serde_json::Value> {
    let folder = std::thread::spawn(|| {
        rfd::FileDialog::new().set_title("选择下载目录").pick_folder()
    }).join().ok().flatten();
    match folder {
        Some(f) => Json(serde_json::json!({"path": f.to_string_lossy()})),
        None => Json(serde_json::json!({"path": null})),
    }
}

async fn get_bilibili_cookie(State(state): State<AppState>) -> Json<serde_json::Value> {
    let cfg = state.mgr.config.read().await;
    let has = !cfg.bilibili_cookie.is_empty();
    let masked = if has { "已设置" } else { "" };
    Json(serde_json::json!({"has_cookie": has, "cookie_masked": masked}))
}

async fn set_bilibili_cookie(
    State(state): State<AppState>,
    Json(req): Json<serde_json::Value>,
) -> Json<serde_json::Value> {
    let cookie = req.get("cookie").and_then(|v| v.as_str()).unwrap_or("");
    let mut cfg = state.mgr.config.write().await;
    cfg.bilibili_cookie = cookie.to_string();
    let _ = cfg.save();
    Json(serde_json::json!({"status": "ok"}))
}

// ====== 合集 ======
async fn list_collections(State(state): State<AppState>) -> Json<serde_json::Value> {
    let cols = state.collections.read().await;
    let list: Vec<&CollectionInfo> = cols.values().collect();
    Json(serde_json::json!(list))
}

async fn create_collection(
    State(state): State<AppState>,
    Json(req): Json<CollectionCreateReq>,
) -> Json<serde_json::Value> {
    let id = uuid::Uuid::new_v4().to_string();
    let title = req.title.unwrap_or_else(|| "合集".into());
    let videos: Vec<CollectionVideo> = req.videos.unwrap_or_default().iter().enumerate().map(|(_i, v)| {
        CollectionVideo {
            title: v.get("title").and_then(|t| t.as_str()).unwrap_or("").into(),
            author: v.get("author").and_then(|a| a.as_str()).unwrap_or("").into(),
            cover_url: v.get("cover_url").and_then(|c| c.as_str()).unwrap_or("").into(),
            url: v.get("url").and_then(|u| u.as_str()).unwrap_or("").into(),
            status: "pending".into(),
            progress: 0,
            speed: 0,
            file_path: String::new(),
            error_message: String::new(),
        }
    }).collect();
    let total = videos.len() as i32;
    let col = CollectionInfo {
        id: id.clone(), url: req.url, title, author: String::new(), cover_url: String::new(),
        total_count: total, videos, status: "downloading".into(),
        quality: req.quality.unwrap_or_else(|| "1080p".into()),
        created_at: chrono::Utc::now().to_rfc3339(),
    };
    // 为每个选中的视频创建下载任务
    let indices = req.selected_indices.unwrap_or_else(|| (0..total as usize).collect());
    for &i in &indices {
        if let Some(v) = col.videos.get(i) {
            if !v.url.is_empty() {
                let task = state.mgr.create_task(&v.url, &col.quality).await;
                let tid = task.id.clone();
                let mgr = state.mgr.clone();
                tokio::spawn(async move { mgr.execute_task(&tid).await.ok(); });
            }
        }
    }
    state.collections.write().await.insert(id.clone(), col);
    Json(serde_json::json!({"id": id, "status": "ok"}))
}

async fn get_collection(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    let cols = state.collections.read().await;
    if let Some(col) = cols.get(&id) {
        // 更新合集内视频状态
        let tasks = state.mgr.tasks.read().await;
        let mut col = col.clone();
        for v in &mut col.videos {
            for t in tasks.values() {
                if t.title == v.title || t.url == v.url {
                    v.status = match &t.status {
                        crate::models::TaskStatus::Pending => "pending",
                        crate::models::TaskStatus::Parsing => "downloading",
                        crate::models::TaskStatus::Downloading => "downloading",
                        crate::models::TaskStatus::Completed => "completed",
                        crate::models::TaskStatus::Failed => "failed",
                    }.into();
                    v.progress = t.progress;
                    v.file_path = t.file_path.clone();
                    v.error_message = t.error_message.clone();
                    break;
                }
            }
        }
        Ok(Json(serde_json::json!(col)))
    } else {
        Err((StatusCode::NOT_FOUND, "合集不存在".into()))
    }
}

async fn delete_collection(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Json<serde_json::Value> {
    state.collections.write().await.remove(&id);
    Json(serde_json::json!({"status": "ok"}))
}

async fn download_collection(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
) -> Json<serde_json::Value> {
    let cols = state.collections.read().await;
    if let Some(col) = cols.get(&id) {
        for v in &col.videos {
            if !v.url.is_empty() && v.status != "completed" {
                let task = state.mgr.create_task(&v.url, &col.quality).await;
                let tid = task.id.clone();
                let mgr = state.mgr.clone();
                tokio::spawn(async move { mgr.execute_task(&tid).await.ok(); });
            }
        }
    }
    Json(serde_json::json!({"status": "ok"}))
}

// ====== 日志 ======
async fn get_logs(State(state): State<AppState>) -> Json<serde_json::Value> {
    let logs = state.logs.read().await;
    Json(serde_json::json!(*logs))
}

async fn clear_logs(State(state): State<AppState>) -> Json<serde_json::Value> {
    state.logs.write().await.clear();
    Json(serde_json::json!({"status": "ok"}))
}

// ====== 图片代理 ======
async fn proxy_image(Query(params): Query<HashMap<String, String>>) -> Response {
    let url = match params.get("url") {
        Some(u) => u,
        None => return (StatusCode::BAD_REQUEST, "missing url").into_response(),
    };
    let client = reqwest::Client::new();
    let resp = match client.get(url)
        .header("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
        .header("Referer", "https://www.douyin.com/")
        .send().await
    {
        Ok(r) => r,
        Err(e) => return (StatusCode::BAD_GATEWAY, e.to_string()).into_response(),
    };
    let ct = resp.headers().get("content-type").and_then(|v| v.to_str().ok()).unwrap_or("image/jpeg").to_string();
    let bytes = match resp.bytes().await {
        Ok(b) => b,
        Err(e) => return (StatusCode::BAD_GATEWAY, e.to_string()).into_response(),
    };
    (StatusCode::OK, [("Content-Type", ct)], bytes.to_vec()).into_response()
}

// ====== 兼容接口 ======
async fn console_visible() -> Json<serde_json::Value> {
    Json(serde_json::json!({"visible": false}))
}

async fn console_toggle() -> Json<serde_json::Value> {
    Json(serde_json::json!({"status": "ok"}))
}

async fn check_update() -> Json<serde_json::Value> {
    Json(serde_json::json!({"message": "已是最新版本", "version": env!("CARGO_PKG_VERSION")}))
}

// ====== 从预览创建任务 ======
async fn create_from_preview(
    State(state): State<AppState>,
    Json(req): Json<serde_json::Value>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    let url = req.get("url").and_then(|v| v.as_str()).unwrap_or("").to_string();
    let quality = req.get("quality").and_then(|v| v.as_str()).unwrap_or("1080p").to_string();
    if url.is_empty() { return Err((StatusCode::BAD_REQUEST, "url required".into())); }
    let task = state.mgr.create_task(&url, &quality).await;
    let id1 = task.id.clone();
    let id2 = task.id.clone();
    let mgr = state.mgr.clone();
    tokio::spawn(async move { mgr.execute_task(&id1).await.ok(); });
    Ok(Json(serde_json::json!({"id": id2, "status": "ok"})))
}

// ====== 合集订阅 ======
#[derive(Deserialize)]
struct SubscribeReq { subscribe: Option<bool>, refresh_interval: Option<u64> }

async fn subscribe_collection(
    State(state): State<AppState>,
    AxumPath(id): AxumPath<String>,
    Json(req): Json<SubscribeReq>,
) -> Json<serde_json::Value> {
    let mut cols = state.collections.write().await;
    if let Some(col) = cols.get_mut(&id) {
        if req.subscribe.unwrap_or(false) {
            col.status = "subscribed".into();
        } else {
            col.status = "idle".into();
        }
    }
    Json(serde_json::json!({"status": "ok"}))
}

// ====== 合集内单视频下载 ======
async fn download_collection_video(
    State(state): State<AppState>,
    AxumPath((id, idx)): AxumPath<(String, usize)>,
) -> Result<Json<serde_json::Value>, (StatusCode, String)> {
    let cols = state.collections.read().await;
    let col = cols.get(&id).ok_or_else(|| (StatusCode::NOT_FOUND, "合集不存在".into()))?;
    let v = col.videos.get(idx).ok_or_else(|| (StatusCode::BAD_REQUEST, "索引越界".into()))?;
    if v.url.is_empty() { return Err((StatusCode::BAD_REQUEST, "视频URL为空".into())); }
    let task = state.mgr.create_task(&v.url, &col.quality).await;
    let tid1 = task.id.clone();
    let tid2 = task.id.clone();
    let mgr = state.mgr.clone();
    tokio::spawn(async move { mgr.execute_task(&tid1).await.ok(); });
    Ok(Json(serde_json::json!({"status": "ok", "task_id": tid2})))
}

// ====== 删除合集内视频文件 ======
async fn delete_collection_video_file(
    State(state): State<AppState>,
    AxumPath((id, idx)): AxumPath<(String, usize)>,
) -> Json<serde_json::Value> {
    let mut cols = state.collections.write().await;
    if let Some(col) = cols.get_mut(&id) {
        if let Some(v) = col.videos.get_mut(idx) {
            if !v.file_path.is_empty() {
                let _ = std::fs::remove_file(&v.file_path);
                v.file_path.clear();
                v.status = "pending".into();
                v.progress = 0;
            }
        }
    }
    Json(serde_json::json!({"status": "ok"}))
}

// ====== B站扫码登录 (placeholder) ======
async fn bilibili_login() -> Json<serde_json::Value> {
    Json(serde_json::json!({"message": "请在设置中手动粘贴Cookie", "status": "manual"}))
}



