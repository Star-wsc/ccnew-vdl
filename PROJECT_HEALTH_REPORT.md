# CCNEW-VideoDownloader 项目健康报告

> 更新时间：2026-08-28
> 当前版本：v1.3.4（`main` 分支，tag `v1.3.4`）
> 最新提交：`ef59bf2`（fix: Go版弹窗修复 + 前端版本比较修复）
> 本地 Go：1.26.3 windows/amd64

---

## 一、项目概览

Go + Gin 后端 + 单文件 HTML 前端，WebView2 桌面壳，Docker 多架构发布，GitHub Actions CI 自动打包。支持抖音/B站视频解析下载、合集批量下载、订阅刷新。

### 代码规模

| 文件 | 行数 | 说明 |
|------|------|------|
| Go 源码（25个文件） | ~7744 行 | 含2个测试文件 |
| `static/index.html` | 5850 行 | 经典UI |
| `static/index-v2.html` | 608 行 | 新版UI（深色终端风，默认） |

### 分支状态

| 分支 | 状态 | 说明 |
|------|------|------|
| `main` | ✅ 生产 | Go + WebView2，Windows桌面版 + Docker版 |
| `native-windows` | ⚠️ 废弃 | Rust (tao+wry+axum) 实验，下载性能差已放弃 |
| `csharp-wpf` | ⚠️ 废弃 | C# WinUI3 实验，UI框架搭建失败已放弃 |

### 架构

```
┌─ Windows 桌面版 ─────────────────────────────────┐
│ CCNEW-VideoDownloader.exe                         │
│   ├── Gin HTTP :18000                             │
│   ├── 生成 ccnew-vdl-window.ps1                   │
│   └── PS1 启动 WebView2 → 加载 index-v2.html      │
│        （URL 带时间戳参数 ?t=xxx 防缓存）           │
└───────────────────────────────────────────────────┘

┌─ Docker / NAS 版 ────────────────────────────────┐
│ server (linux/amd64 或 linux/arm64)               │
│   ├── Gin HTTP :18000 → 浏览器访问                │
│   ├── volumes: /downloads, /logs                  │
│   └── env: DOWNLOAD_DIR, PORT, BILIBILI_COOKIE... │
└───────────────────────────────────────────────────┘

internal/
  bilibili/       单视频解析、下载、M4S剥离+FFmpeg合并、WBI签名、6类合集
  douyin/         单视频5策略降级、合集(普通/短剧)、音视频分离+FFmpeg合并
  download/       任务管理器：创建/执行/进度/JSON原子持久化
  config/         环境变量 → config.json → 默认值
  fsutil/         原子写入工具（临时文件+rename）
  models/         数据模型

cmd/server/
  main.go                    路由、CORS、端口管理、信号处理
  handlers.go                全部API处理（~1737行，59KB）
  launcher_windows.go        WebView2桌面窗口启动、PS1生成、进程管理
  launcher_linux.go          Linux空实现（Docker不启动桌面窗口）
  console_windows.go         控制台窗口隐藏
  proxy_guard_test.go        SSRF防护测试（15个用例）
```

---

## 二、核心踩坑记录（必读）

> **以下每个问题都是真实踩过的坑，后续开发必须注意。**

### 坑1：抖音视频下载后没有声音

**现象**：下载的抖音视频能播放画面，但没有声音。有的有声有的没声，没有规律。

**根因**：抖音使用 DASH 格式，视频流和音频流是分开的两个独立文件。早期版本只下载了视频流，没有下载音频流，也没有合并。

**修复**：
- 在 `internal/douyin/downloader.go` 中增加 `enrichAudioURL()` 方法，从 API 响应中提取音频流 URL
- 在 `internal/download/manager.go` 中增加双流下载逻辑：先下载视频流到 `.tmp`，再下载音频流到 `.audio.tmp`，最后用 FFmpeg 合并
- 合并命令：`ffmpeg -i video.m4s -i audio.m4s -c:v copy -c:a copy -y output.mp4`

**教训**：
- 抖音的 `play_addr` 只包含视频流，音频在 `bit_rate_audio` 字段中
- 不同视频的 API 响应结构不一致，有的有 `bit_rate_audio`，有的没有，需要多策略兜底
- 合并前必须检查 FFmpeg 是否可用，否则降级为只下载视频流

---

### 坑2：WebView2 窗口显示旧版界面（缓存问题）

**现象**：更新了 `index.html` 或 `index-v2.html` 后，桌面窗口打开的还是旧版界面。刷新也没用。

**根因**：WebView2 会持久化缓存页面。桌面窗口只在启动时加载一次 URL，后续 HTML 文件更新不会自动刷新已打开的窗口。WebView2 的缓存目录在 `%TEMP%\ccnew-vdl-wv2`。

**修复**：
- 启动器 URL 加时间戳参数：`http://127.0.0.1:18000/?t={timestamp}`，每次启动强制新导航
- `launcher_windows.go` 的 `generatePS1()` 函数中生成带时间戳的 URL

**教训**：
- WebView2 不等于浏览器刷新，它是嵌入式组件，缓存行为不同于 F5
- 如果需要更新前端，必须重启整个程序（杀掉 server + WebView2 进程）
- 开发时如果频繁改 HTML，可以手动删除 `%TEMP%\ccnew-vdl-wv2` 目录

---

### 坑3：版本号比较失败（v前缀不一致）

**现象**：明明已经是最新版本，但界面一直提示"发现新版本"。

**根因**：GitHub tag 格式是 `v1.3.4`（带 `v` 前缀），但本地编译的版本号是 `1.3.4`（不带 `v`）。直接字符串比较 `latestTag === currentVersion` 永远不等。

**修复**：`static/index.html` 中比较前统一去掉 `v` 前缀：
```javascript
const normalizedLatest = latestTag.replace(/^v/, "");
const normalizedCurrent = currentVersion.replace(/^v/, "");
```

**教训**：
- 版本号格式必须统一。建议所有地方都存不带 `v` 的版本号，展示时再加
- GitHub Actions 构建时用 `${{ github.ref_name }}` 注入版本号，tag 名自带 `v`，需要在 ldflags 中剥离或在前端比较时剥离
- `Dockerfile` 中 `ARG VERSION=v1.3.4` 也带 `v`，需要保持一致

---

### 坑4：控制台窗口闪烁（subprocess 弹黑框）

**现象**：程序运行过程中会突然弹出黑色命令提示符窗口，闪一下就消失。

**根因**：Windows 上用 `exec.Command()` 启动子进程时，默认会创建一个新的控制台窗口。调用 `tasklist`、`taskkill`、`powershell` 等命令都会弹窗。

**修复**：封装 `silentCmd()` 函数，所有子进程调用都用它：
```go
func silentCmd(name string, args ...string) *exec.Cmd {
    cmd := exec.Command(name, args...)
    cmd.SysProcAttr = &syscall.SysProcAttr{
        HideWindow:    true,
        CreationFlags: 0x08000000, // CREATE_NO_WINDOW
    }
    return cmd
}
```

**教训**：
- Windows 上任何 `exec.Command()` 调用都可能弹窗，必须统一用 `silentCmd()`
- 主程序编译时加 `-H windowsgui` ldflags 隐藏主控制台窗口
- PowerShell 脚本启动时加 `-WindowStyle Hidden` 参数

---

### 坑5：tasks.json / collections.json 数据损坏

**现象**：程序崩溃或断电后，任务列表丢失或 JSON 解析报错。

**根因**：直接用 `os.WriteFile()` 写入 JSON 文件。如果写入过程中程序被杀（崩溃、断电、用户强杀），文件会写到一半，变成损坏的 JSON。

**修复**：新增 `internal/fsutil/fsutil.go`，实现原子写入：
1. 写入临时文件 `xxx.tmp`
2. `os.Rename()` 替换原文件（操作系统保证 rename 是原子的）

```go
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, perm); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

**教训**：
- 任何持久化文件（JSON、config）都不能直接 `WriteFile`，必须原子写入
- Windows 上 `os.Rename()` 如果目标文件被其他进程打开会失败，需要先关闭文件句柄
- 程序启动时应该能容忍 JSON 文件不存在或损坏，降级为空数据

---

### 坑6：WebView2 进程清理误杀其他应用

**现象**：关闭程序后，其他使用 WebView2 的应用（如 Electron 应用）也被杀了。

**根因**：`killChildProcess()` 中用了 `taskkill /F /IM msedgewebview2.exe`，这会杀掉系统上所有 WebView2 进程，不只是本程序启动的。

**修复**：
- `cleanupOrphans()` 只杀命令行中包含 `ccnew-vdl-wv2` 缓存目录的 WebView2 进程
- 通过 `tasklist /FI "IMAGENAME eq msedgewebview2.exe" /FO CSV` 获取 PID，再用 `wmic` 检查命令行参数过滤

**教训**：
- 永远不要用 `/IM` 按进程名强杀，必须用 `/PID` 按进程ID杀
- 启动子进程时记录 PID，退出时按 PID 清理
- WebView2 是共享组件，系统上可能有多个应用在用

---

### 坑7：端口占用检测误杀无关进程

**现象**：启动时把占用 18000 端口的其他程序（如 Redis、其他 Web 服务）杀了。

**根因**：早期版本用 `netstat` 找到占用端口的 PID 后直接 `taskkill`，不检查那个进程是什么程序。

**修复**：`killPortProcess()` 增加进程名匹配：
- 获取当前可执行文件名（`selfName`）
- 用 `tasklist /FI "PID eq xxx"` 获取占用端口的进程名
- 只杀进程名与自身相同的旧实例，其他进程只打日志不杀

**教训**：
- 端口 18000 不是特权端口，任何程序都可能占用
- Docker 环境中端口映射到宿主机，宿主机上可能有其他服务占用
- 启动失败时应该报错让用户手动处理，而不是自动杀进程

---

### 坑8：config.json 覆盖环境变量（Docker 路径问题）

**现象**：Docker 版设置了 `DOWNLOAD_DIR=/downloads`，但实际下载到了容器内其他路径。

**根因**：config 加载顺序是 `默认值 → 环境变量 → config.json`。config.json 在最后加载，会覆盖环境变量。用户在 UI 中修改下载目录后，config.json 记录了新路径。容器重启时 config.json 的路径覆盖了环境变量。

**修复**（2026-08-28 已实现）：调整 `internal/config/config.go` 加载顺序为 `默认值 → config.json → 环境变量`，环境变量优先级最高。Docker 下 compose 声明的 `DOWNLOAD_DIR`/`PORT`/Cookie 不再被 config.json 覆盖；桌面版不设环境变量，config.json 仍正常生效。同时补上此前从未被读取的 `LOG_DIR` 环境变量。单测：`internal/config/config_test.go`。

**教训**：
- 配置优先级设计要考虑容器化场景。容器内路径和宿主机路径不同，config.json 记录的是容器内路径
- Docker 中应该只用环境变量配置，不要持久化 config.json
- `docker-compose.yml` 中挂载 `./config:/root/.config/ccnew-vdl` 会导致 config.json 跨重启持久化

---

### 坑9：SSRF 代理漏洞

**现象**：`/api/proxy/image?url=xxx` 接口可以被用来扫描内网或访问任意 URL。

**根因**：图片代理接口没有校验目标 URL，攻击者可以构造 `?url=http://169.254.169.254/latest/meta-data/` 访问云服务元数据，或 `?url=http://192.168.1.1/admin` 扫描内网。

**修复**：三层防护：
1. 域名后缀白名单（仅允许 `hdslb.com`、`douyinpic.com`、`byteimg.com` 等媒体 CDN）
2. 重定向逐跳校验（防止302跳转到内网）
3. 建连前解析 DNS 并检查 IP 是否为私网/回环/链路本地

**教训**：
- 任何代理/转发接口都是 SSRF 攻击面，必须做白名单校验
- 不能只校验初始 URL，还要校验重定向后的最终 URL
- DNS rebinding 攻击可以在校验后、建连前切换 IP，所以要在建连前再检查一次

---

### 坑10：存储型 XSS

**现象**：在经典 UI 中，任务标题包含 `<script>` 标签时会执行。

**根因**：抖音/B站返回的视频标题、作者名等用户输入，直接用 `innerHTML` 插入到页面中，没有转义。

**修复**：
- 新版 UI (`index-v2.html`) 所有动态文本都经 `esc()` 函数转义
- `esc()` 将 `<>&"'` 替换为 HTML 实体
- 封面图片 URL 走白名单代理，不直接插入

**教训**：
- 任何从外部 API 获取的文本都是不可信的，必须转义后再渲染
- 用 `textContent` 代替 `innerHTML` 是最安全的方式
- 视频标题可能包含 emoji、特殊字符、甚至恶意脚本

---

### 坑11：Rust 版下载性能远不及 Go

**现象**：同一个视频，Go 版秒下，Rust 版要等几十秒甚至超时。

**根因**：Rust 的 `reqwest` 库（基于 tokio）在大文件下载场景下，I/O 调度和缓冲策略与 Go 的 `net/http` 差距很大。Go 的 goroutine 调度和 `net/http` 对长连接、流式读取有极深优化。

**尝试过的方案**：
1. `reqwest` async → 慢
2. `reqwest` blocking → 更慢
3. 原生 `TcpStream` + `native_tls` → 仍然慢5倍
4. 启动 Go 子进程做下载 → 性能接近但引入管道死锁、JSON 编码问题

**最终结论**：放弃 Rust 方案，Windows 版继续用 Go。

**教训**：
- Go 的 `net/http` 在 HTTP 下载场景下性能极强，是 Go 的核心优势之一
- 不要轻易重写已经跑通的核心逻辑，除非有明确的性能或架构收益
- 语言选型要考虑核心场景的库生态，不是"更底层=更快"

---

### 坑12：B站 M4S 文件头剥离

**现象**：B站 DASH 格式下载的 m4s 文件用普通播放器无法播放。

**根因**：B站的 m4s 文件在标准 MP4 容器前加了一段自定义头部数据，需要剥离后才能被 FFmpeg 正确识别。

**修复**：下载后用代码检测并剥离非标准头部，然后用 FFmpeg 合并视频流和音频流。

**教训**：
- B站的 DASH 格式不是标准 DASH，有自定义魔改
- 不能假设下载的文件就是标准 MP4，需要验证文件头
- 合并前先检查 FFmpeg 是否可用，Docker 版需要在镜像中安装 FFmpeg

---

### 坑13：抖音反爬策略频繁变化

**现象**：抖音解析隔一段时间就失效，返回空数据或403。

**根因**：抖音的反爬策略持续升级，包括：
- 短链接需要 `ttwid` cookie 才能重定向
- API 接口需要特定的 User-Agent 和 Referer
- `RENDER_DATA` 字段格式变化
- 合集接口翻页行为变化

**修复**：5策略降级机制：
1. `iesdouyin.com` 分享页 + 移动端 UA
2. `douyin.com` + 移动端 UA
3. 桌面 UA 快速解析（RENDER_DATA + Detail API + HTML Regex）
4. 多 UA 尝试
5. 第三方 API 兜底

**教训**：
- 抖音反爬是持续对抗，不能依赖单一策略
- 每个策略都要有失败后的降级路径
- Cookie 机制是关键，引导用户配置 Cookie 可以大幅提升成功率
- 合集翻页接口不稳定，需要防御式编程（第2页起失败就保留已有结果）

---

## 三、已修复问题清单

### 安全修复

| 编号 | 内容 | 根因 | 状态 |
|------|------|------|------|
| S1 | CORS 精确匹配 | 旧版前缀匹配可被域名伪装绕过 | ✅ |
| S2 | SSRF 代理三层防护 | 代理接口无 URL 校验 | ✅ |
| S3 | XSS 转义 | 外部文本直接 innerHTML | ✅ |
| S4 | 更新 SHA256 校验 | 下载后不校验完整性 | ✅ |
| S5 | config.json 0600 权限 | 含 Cookie 的配置文件全局可读 | ✅ |
| S6 | Cookie 临时文件即用即删 | 登录过程的 Cookie 文件残留 | ✅ |

### 功能修复

| 编号 | 内容 | 根因 | 状态 |
|------|------|------|------|
| F1 | 抖音音视频合并 | DASH 格式音视频分离，只下了视频流 | ✅ |
| F2 | 抖音合集翻页 | 接口不翻页，只返回首页 | ✅(防御式) |
| F3 | JSON 原子写入 | 直接写文件，崩溃时数据损坏 | ✅ |
| F4 | 启动时清理临时文件 | 崩溃遗留的 m4s 碎片堆积 | ✅ |
| F5 | 端口检测不误杀 | 占用端口就杀，不区分进程 | ✅ |
| F6 | 版本号构建注入 | 硬编码版本号，发布时容易漏改 | ✅ |
| F7 | HTML 探测越界修复 | `buf[:n])[:5]` 在 n<5 时 panic | ✅ |
| F8 | 版本比较 v前缀兼容 | tag 带 v，本地不带，比较永远不等 | ✅ |
| F9 | 控制台窗口闪烁 | exec.Command 默认弹窗 | ✅ |
| F10 | WebView2 误杀其他应用 | taskkill /IM 杀所有同名进程 | ✅ |
| F11 | Docker 环境变量被 config.json 覆盖 | 配置加载顺序：config.json 在 env 之后 | ✅ 2026-08-28 |

### UI 重构

| 内容 | 状态 |
|------|------|
| 新版 UI (`index-v2.html`) 深色终端风 | ✅ |
| 默认展示新版 UI | ✅ |
| WebView2 缓存绕过（URL 时间戳） | ✅ |
| 经典 UI 切换按钮 | ✅ |

---

## 四、待修复问题

### handlers.go 上帝文件【优先级：中】

59KB / ~1737 行，所有 API 逻辑在一个文件。建议拆分为 task、collection、config、proxy 四个文件。

### 测试覆盖不足【优先级：中】

仅 2 个测试文件（proxy_guard、fsutil）。核心解析逻辑无测试，回归全靠人工验证。

### WebView2 进程清理仍有隐患【优先级：低】

`launcher_windows.go:95` 的 `killChildProcess()` 仍用 `taskkill /F /IM msedgewebview2.exe` 杀所有 WebView2 进程。`cleanupOrphans()` 已修复但 `killChildProcess()` 未同步修复。

---

## 五、开发规则（必须遵守）

### 1. 前端开发

- **所有外部文本必须转义**：从 API 获取的标题、作者名、URL 等，渲染前必须经 `esc()` 处理
- **封面图片走代理**：不直接插入外部图片 URL，走 `/api/proxy/image` 白名单代理
- **版本比较统一处理**：比较前去掉 `v` 前缀
- **WebView2 缓存**：修改 HTML 后必须重启程序才能看到效果，开发时可删 `%TEMP%\ccnew-vdl-wv2`

### 2. 后端开发

- **子进程调用统一用 `silentCmd()`**：Windows 上任何 `exec.Command()` 都会弹窗
- **文件写入用 `fsutil.AtomicWrite()`**：不直接 `os.WriteFile()`
- **端口操作只杀自己**：`killPortProcess()` 必须检查进程名，不能按端口盲杀
- **进程清理按 PID**：不用 `/IM` 按进程名杀，用 `/PID` 精确杀
- **抖音解析多策略降级**：任何单一策略都可能失效，必须有 fallback

### 3. Docker 开发

- **环境变量优先级最高**：Docker 中不应该被 config.json 覆盖
- **镜像必须包含 FFmpeg**：B站 DASH 合并和抖音音视频合并都需要
- **下载目录用 /downloads**：不要用容器内其他路径，保持与 docker-compose.yml 一致
- **不要挂载 config volume**：或确保 config.json 中的路径与容器内路径一致

### 4. 构建与发布

- **版本号统一管理**：`main.go` 默认值、Dockerfile ARG、installer.iss、CI ldflags 必须同步
- **构建时注入版本号**：`go build -ldflags "-X main.Version=xxx"`
- **Windows 加 `-H windowsgui`**：隐藏主控制台窗口
- **CI 用 tag 名注入版本**：`${{ github.ref_name }}` 会带 `v` 前缀，前端比较时需要剥离

### 5. 不要做的事

- ❌ 不要在 Go 项目中引入 Python（用户明确禁止）
- ❌ 不要用 `taskkill /IM` 杀进程（会误杀其他应用）
- ❌ 不要直接写文件（用原子写入）
- ❌ 不要用 `innerHTML` 插入外部文本（用 `textContent` 或 `esc()`）
- ❌ 不要假设抖音接口稳定（必须多策略降级）
- ❌ 不要在 Docker 中持久化 config.json（环境变量优先）
- ❌ 不要轻易重写核心下载逻辑（Go net/http 性能极强，Rust reqwest 远不如）

---

## 六、构建与部署

### Windows 本地构建

```bash
cd cmd/server
go build -ldflags="-s -w -H windowsgui -X main.Version=1.3.4" -o CCNEW-VideoDownloader.exe .
```

部署到：`F:\迅雷云盘\抖音B站视频解析工具\`

必需文件：
```
CCNEW-VideoDownloader.exe    ← 主程序
static/index.html            ← 经典UI
static/index-v2.html         ← 新版UI（默认）
static/BDYD.ico              ← 图标
webview2/                    ← WebView2 SDK DLL（3个）
ffmpeg/                      ← FFmpeg（B站合并用）
```

桌面快捷方式：指向 `CCNEW-VideoDownloader.exe`，图标用 `BDYD.ico`

### Docker 构建

```bash
docker build -t wsc768043912/ccnew-vdl:latest .
docker push wsc768043912/ccnew-vdl:latest
```

### GitHub Actions 自动发布

触发：推送 `v*` tag

产物：
- `ccnew-vdl-windows-amd64.zip`（含 server.exe + static/ + webview2/ + ffmpeg/）
- `ccnew-vdl-linux-amd64.tar.gz`
- `ccnew-vdl-linux-arm64.tar.gz`
- `ccnew-vdl-setup.exe`（Inno Setup 安装包 + SHA256）

---

## 七、安全现状

| 维度 | 修复前 | 修复后 |
|------|--------|--------|
| API 鉴权 | 无 | 无（局域网部署仍有风险） |
| CORS | 前缀匹配 | 含端口精确匹配 |
| SSRF | 无防护 | 域名白名单+重定向复验+IP黑名单 |
| XSS | 存储型 | 动态文本 esc() 转义 |
| Cookie | 明文全局可读 | 0600 权限 |
| 更新 | 无校验 | SHA256 校验 |
| 端口 | 盲杀占用进程 | 只杀同名旧实例 |

仍建议：局域网部署时添加 API Token 鉴权中间件。