package main

import "os"

func main() {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>视频下载器</title>
<style>
:root{--primary:#6366f1;--primary-dark:#4f46e5;--primary-light:#818cf8;--success:#22c55e;--warning:#f59e0b;--danger:#ef4444;--bg:#0f0f23;--bg-card:#1a1a2e;--bg-sidebar:#16162a;--bg-input:#1e1e3a;--text:#e2e8f0;--text-secondary:#94a3b8;--text-muted:#64748b;--border:#2d2d4a;--border-light:#3d3d5a;--radius:12px;--radius-sm:8px;--transition:all .2s ease}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:var(--bg);color:var(--text);min-height:100vh;display:flex;flex-direction:column}
.header{background:var(--bg-card);border-bottom:1px solid var(--border);padding:12px 24px;display:flex;justify-content:space-between;align-items:center}
.header-title{font-size:18px;font-weight:700;background:linear-gradient(135deg,var(--primary),var(--primary-light));-webkit-background-clip:text;-webkit-text-fill-color:transparent}
.header-actions{display:flex;gap:8px}
.header-btn{padding:8px 16px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-secondary);font-size:13px;cursor:pointer;transition:var(--transition)}
.header-btn:hover{background:var(--border);color:var(--text)}
.header-btn.primary{background:var(--primary);border-color:var(--primary);color:#fff}
.header-btn.primary:hover{background:var(--primary-dark)}
.main{display:flex;flex:1;overflow:hidden}
.sidebar{width:360px;background:var(--bg-sidebar);border-right:1px solid var(--border);display:flex;flex-direction:column;overflow-y:auto}
.content{flex:1;display:flex;flex-direction:column;overflow:hidden}
.content-header{padding:16px 24px;border-bottom:1px solid var(--border)}
.content-body{flex:1;overflow-y:auto;padding:16px 24px}
.sidebar-section{padding:16px;border-bottom:1px solid var(--border)}
.section-title{font-size:13px;font-weight:600;color:var(--text-secondary);text-transform:uppercase;letter-spacing:.5px;margin-bottom:12px}
.url-input{width:100%;padding:12px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text);font-size:14px;resize:vertical;min-height:80px;outline:none;transition:var(--transition)}
.url-input:focus{border-color:var(--primary)}
.url-input::placeholder{color:var(--text-muted)}
.quality-select{width:100%;padding:10px 12px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text);font-size:13px;outline:none;cursor:pointer;margin-top:8px}
.btn{padding:10px 20px;border:none;border-radius:var(--radius-sm);font-size:13px;font-weight:600;cursor:pointer;transition:var(--transition);display:inline-flex;align-items:center;justify-content:center;gap:6px}
.btn-primary{background:var(--primary);color:#fff;width:100%;margin-top:8px}
.btn-primary:hover{background:var(--primary-dark)}
.btn-primary:disabled{background:#4b5563;cursor:not-allowed}
.btn-sm{padding:6px 12px;font-size:12px}
.btn-danger{background:transparent;color:var(--danger);border:1px solid var(--danger)}
.btn-danger:hover{background:var(--danger);color:#fff}
.btn-success{background:var(--success);color:#fff}
.btn-success:hover{background:#16a34a}

/* Mode Tabs */
.mode-tabs{display:flex;gap:0;margin-bottom:12px}
.mode-tab{flex:1;padding:10px 16px;background:var(--bg-input);border:1px solid var(--border);color:var(--text-secondary);font-size:13px;font-weight:600;cursor:pointer;transition:var(--transition);text-align:center}
.mode-tab:first-child{border-radius:var(--radius-sm) 0 0 var(--radius-sm)}
.mode-tab:last-child{border-radius:0 var(--radius-sm) var(--radius-sm) 0}
.mode-tab:hover{color:var(--text)}
.mode-tab.active{background:var(--primary);border-color:var(--primary);color:#fff}

/* Filter Tabs */
.filter-tabs{display:flex;gap:4px}
.filter-tab{padding:6px 14px;background:transparent;border:1px solid var(--border);border-radius:20px;color:var(--text-secondary);font-size:12px;cursor:pointer;transition:var(--transition)}
.filter-tab:hover{border-color:var(--primary);color:var(--text)}
.filter-tab.active{background:var(--primary);border-color:var(--primary);color:#fff}
.content-header-row{display:flex;justify-content:space-between;align-items:center}
.stats{font-size:12px;color:var(--text-secondary)}
.stat-value{font-weight:600;color:var(--text)}

/* Task List */
.task-list{display:flex;flex-direction:column;gap:8px}
.task-item{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:16px;cursor:pointer;transition:var(--transition)}
.task-item:hover{border-color:var(--border-light)}
.task-header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:8px}
.task-title{font-weight:500;font-size:14px;line-height:1.4}
.task-author{font-size:12px;color:var(--text-secondary);margin-top:4px}
.task-url{font-size:11px;color:var(--text-muted);margin-top:4px;word-break:break-all}
.status-badge{font-size:11px;padding:3px 10px;border-radius:20px;font-weight:500;white-space:nowrap}
.status-pending{background:rgba(245,158,11,.15);color:var(--warning)}
.status-parsing{background:rgba(99,102,241,.15);color:var(--primary-light)}
.status-downloading{background:rgba(34,197,94,.15);color:var(--success)}
.status-completed{background:rgba(34,197,94,.15);color:var(--success)}
.status-failed{background:rgba(239,68,68,.15);color:var(--danger)}
.progress-bar{height:4px;background:var(--bg-input);border-radius:2px;overflow:hidden;margin-top:8px}
.progress-fill{height:100%;background:linear-gradient(90deg,var(--primary),var(--primary-light));border-radius:2px;transition:width .3s}
.error-msg{font-size:12px;color:var(--danger);margin-top:8px;padding:6px 10px;background:rgba(239,68,68,.1);border-radius:var(--radius-sm)}
.task-actions{display:flex;gap:6px;margin-top:10px}
.task-meta{font-size:12px;color:var(--text-muted);margin-top:6px;display:flex;gap:12px}

/* Collection List */
.collection-list{display:flex;flex-direction:column;gap:8px}
.collection-item{background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:16px;transition:var(--transition)}
.collection-item:hover{border-color:var(--border-light)}
.collection-header{display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:8px}
.collection-title{font-weight:500;font-size:14px}
.collection-meta{font-size:12px;color:var(--text-secondary);margin-top:4px}
.collection-actions{display:flex;gap:6px;margin-top:10px}
.collection-videos{margin-top:12px;border-top:1px solid var(--border);padding-top:12px}
.collection-video{display:flex;gap:12px;padding:8px 0;border-bottom:1px solid var(--border)}
.collection-video:last-child{border-bottom:none}
.collection-video-thumb{width:80px;height:45px;border-radius:4px;object-fit:cover;background:var(--bg-input)}
.collection-video-info{flex:1}
.collection-video-title{font-size:12px;font-weight:500;line-height:1.3}
.collection-video-meta{font-size:11px;color:var(--text-muted);margin-top:4px}

/* Empty State */
.empty-state{text-align:center;padding:60px 20px;color:var(--text-secondary)}
.empty-state .icon{font-size:48px;margin-bottom:16px}
.empty-state .title{font-size:16px;font-weight:600;color:var(--text);margin-bottom:8px}
.empty-state .desc{font-size:13px}

/* Toast */
.toast{position:fixed;top:20px;right:20px;padding:12px 20px;background:var(--bg-card);color:var(--text);border-radius:var(--radius-sm);border:1px solid var(--border);box-shadow:0 10px 15px -3px rgba(0,0,0,.4);display:none;z-index:1000;font-size:13px}
.toast.show{display:block;animation:slideIn .3s ease}
.toast.success{border-color:var(--success)}
.toast.error{border-color:var(--danger)}
@keyframes slideIn{from{transform:translateX(100px);opacity:0}to{transform:translateX(0);opacity:1}}
.loading{display:inline-block;width:14px;height:14px;border:2px solid rgba(255,255,255,.3);border-top-color:#fff;border-radius:50%;animation:spin .8s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}

/* Modal */
.modal{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.6);display:none;z-index:100;align-items:center;justify-content:center}
.modal.show{display:flex}
.modal-content{background:var(--bg-card);border-radius:var(--radius);padding:24px;width:90%;max-width:500px}
.modal-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}
.modal-title{font-size:18px;font-weight:600}
.modal-close{background:none;border:none;color:var(--text-secondary);font-size:24px;cursor:pointer}
.settings-group{margin-bottom:16px}
.settings-label{font-size:13px;color:var(--text-secondary);margin-bottom:6px}
.settings-input{width:100%;padding:10px 12px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text);font-size:13px;outline:none}
</style>
</head>
<body>
<header class="header">
<div class="header-title">🎬 视频下载器</div>
<div class="header-actions">
<button class="header-btn" onclick="clearCompleted()">清理已完成</button>
<button class="header-btn primary" onclick="showSettings()">设置</button>
</div>
</header>
<div class="main">
<aside class="sidebar">
<div class="sidebar-section">
<div class="section-title">添加任务</div>
<textarea class="url-input" id="urlInput" placeholder="粘贴视频链接...&#10;支持抖音、B站单视频和合集链接"></textarea>
<select class="quality-select" id="qualitySelect">
<option value="4k">4K 超高清 (需大会员)</option>
<option value="1080p" selected>1080P 高清</option>
<option value="720p">720P 标清</option>
<option value="480p">480P 流畅</option>
</select>
<button class="btn btn-primary" id="addBtn" onclick="addTask()"><span id="btnText">添加下载任务</span></button>
</div>
</aside>
<div class="content">
<div class="content-header">
<div class="mode-tabs">
<button class="mode-tab active" id="singleModeBtn" onclick="switchMode('single')">单视频任务</button>
<button class="mode-tab" id="collectionModeBtn" onclick="switchMode('collection')">合集任务</button>
</div>
<div class="content-header-row">
<div class="filter-tabs">
<button class="filter-tab active" onclick="setFilter(this,'')">全部</button>
<button class="filter-tab" onclick="setFilter(this,'downloading')">下载中</button>
<button class="filter-tab" onclick="setFilter(this,'completed')">已完成</button>
<button class="filter-tab" onclick="setFilter(this,'failed')">失败</button>
</div>
<div class="stats">共 <span class="stat-value" id="statTotal">0</span> 个</div>
</div>
</div>
<div class="content-body">
<div id="taskList" class="task-list">
<div class="empty-state"><div class="icon">📋</div><div class="title">暂无单视频任务</div><div class="desc">在左侧粘贴视频链接开始下载</div></div>
</div>
<div id="collectionList" class="collection-list" style="display:none">
<div class="empty-state"><div class="icon">📁</div><div class="title">暂无合集任务</div><div class="desc">粘贴合集链接后添加下载</div></div>
</div>
</div>
</div>
</div>
<div class="modal" id="settingsModal">
<div class="modal-content">
<div class="modal-header"><div class="modal-title">设置</div><button class="modal-close" onclick="closeModal('settingsModal')">&times;</button></div>
<div class="settings-group"><div class="settings-label">B站 Cookie (用于4K下载)</div><textarea class="settings-input" id="settingBilibiliCookie" rows="3" placeholder="从浏览器复制Cookie"></textarea></div>
<div class="settings-group"><div class="settings-label">抖音 Cookie</div><textarea class="settings-input" id="settingDouyinCookie" rows="3" placeholder="从浏览器复制Cookie"></textarea></div>
<button class="btn btn-primary" onclick="saveSettings()" style="width:auto">保存</button>
</div>
</div>
<div id="toast" class="toast"></div>
<script>
let tasks=[],collections=[],currentMode='single',currentFilter='';
async function loadTasks(){try{const r=await fetch('/api/tasks');if(r.ok){tasks=await r.json();renderTasks()}}catch(e){}}
async function loadCollections(){try{const r=await fetch('/api/collections');if(r.ok){collections=await r.json();renderCollections()}}catch(e){}}
function switchMode(m){currentMode=m;document.getElementById('singleModeBtn').classList.toggle('active',m==='single');document.getElementById('collectionModeBtn').classList.toggle('active',m==='collection');document.getElementById('taskList').style.display=m==='single'?'flex':'none';document.getElementById('collectionList').style.display=m==='collection'?'flex':'none';if(m==='collection')loadCollections()}
function setFilter(el,f){document.querySelectorAll('.filter-tab').forEach(t=>t.classList.remove('active'));el.classList.add('active');currentFilter=f;if(currentMode==='single')renderTasks();else renderCollections()}
async function addTask(){const i=document.getElementById('urlInput'),q=document.getElementById('qualitySelect').value,b=document.getElementById('addBtn'),t=document.getElementById('btnText'),u=i.value.trim();if(!u){showToast('请输入链接','error');return}b.disabled=true;t.innerHTML='<span class="loading"></span> 解析中...';try{const r=await fetch('/api/tasks',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:u,quality:q})});if(r.ok){i.value='';showToast('任务已创建','success');await loadTasks();if(currentMode==='collection')await loadCollections()}else{const e=await r.json();showToast(e.error||'失败','error')}}catch(e){showToast('网络错误','error')}finally{b.disabled=false;t.textContent='添加下载任务'}}
function renderTasks(){const c=document.getElementById('taskList');let l=currentFilter?tasks.filter(t=>t.status===currentFilter):tasks;if(!l.length){c.innerHTML='<div class="empty-state"><div class="icon">📋</div><div class="title">暂无单视频任务</div><div class="desc">粘贴单视频链接开始下载</div></div>';return}c.innerHTML=l.map(t=>'<div class="task-item"><div class="task-header"><div><div class="task-title">'+(t.title||'解析中...')+'</div>'+(t.author?'<div class="task-author">👤 '+t.author+'</div>':'')+'</div><span class="status-badge status-'+t.status+'">'+st(t.status)+'</span></div><div class="task-url">'+t.url+'</div>'+(t.status==='downloading'?'<div class="progress-bar"><div class="progress-fill" style="width:'+t.progress+'%"></div></div>':'')+(t.error_message?'<div class="error-msg">❌ '+t.error_message+'</div>':'')+'<div class="task-actions">'+(t.status==='completed'?'<button class="btn btn-success btn-sm" onclick="event.stopPropagation();window.open(\'/api/tasks/'+t.id+'/download\')">📥 下载</button>':'')+(t.status==='failed'?'<button class="btn btn-primary btn-sm" onclick="event.stopPropagation();retryTask(\''+t.id+'\')">🔄 重试</button>':'')+'<button class="btn btn-danger btn-sm" onclick="event.stopPropagation();deleteTask(\''+t.id+'\')">🗑 删除</button></div></div>').join('')}
function renderCollections(){const c=document.getElementById('collectionList');if(!collections.length){c.innerHTML='<div class="empty-state"><div class="icon">📁</div><div class="title">暂无合集任务</div><div class="desc">粘贴合集链接后添加下载</div></div>';return}c.innerHTML=collections.map(col=>'<div class="collection-item"><div class="collection-header"><div><div class="collection-title">'+(col.title||'未知合集')+'</div><div class="collection-meta">'+col.total_count+' 个视频 · '+col.status+'</div></div><span class="status-badge status-'+col.status+'">'+st(col.status)+'</span></div><div class="collection-actions"><button class="btn btn-primary btn-sm" onclick="downloadCollection(\''+col.id+'\')">📥 下载全部</button><button class="btn btn-danger btn-sm" onclick="deleteCollection(\''+col.id+'\')">🗑 删除</button></div></div>').join('')}
</script>
</body>
</html>`
	os.WriteFile("static/index.html", []byte(html), 0644)
}
