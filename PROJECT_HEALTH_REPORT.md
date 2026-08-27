# CCNEW-VideoDownloader 深度体检报告

> 体检对象：`github.com/Star-wsc/ccnew-vdl`（本地路径 `D:\coderom\CCNEW-VideoDownloader`）
> 体检时间：2026-08-25
> 当前分支：`main`（与 `origin/main` 同步，工作区干净）
> 最新提交：`7ac1d87`（2026-08-25，fix: 版本号改为构建时注入 + 修复平台检测）
> 报告性质：只读体检，未修改任何业务代码

---

## 一、总体结论

这是一个功能野心不小、落地速度很快的个人项目：Go + Gin 后端，单文件 HTML 前端，WebView2 桌面壳，Docker 多架构发布，CI 自动打包安装器。B站 DASH 下载（M4S 头剥离 + FFmpeg 合并）、抖音多策略解析降级、合集订阅刷新这些核心思路都是对的，很多细节能看出踩坑后的真实经验。

但以"可长期维护、可放心暴露使用"的标准衡量，项目目前处于**功能先行、安全与工程质量欠账较重**的状态：

| 维度 | 评分 (0-10) | 一句话诊断 |
|---|---:|---|
| 架构与模块化 | 6 | 分层意识不错，但 `handlers.go` 已经长成上帝文件 |
| 代码质量 | 5 | 能跑、思路清晰，但有死代码、重复实现、14 个文件未格式化 |
| 测试 | 0 | 全仓库零测试文件，回归全靠人肉 |
| 安全 | 2.5 | 未鉴权 API 监听全网卡 + SSRF 代理 + 存储型 XSS + 更新不校验，是最大短板 |
| 可靠性/并发 | 4 | 共享指针竞态、非原子持久化、孤儿下载、并发上限可被放大 |
| 文档准确性 | 5 | README 有幽灵接口和幽灵目录，版本号三处不一致 |
| 依赖与供应链 | 4 | 核心间接依赖落后多个安全版本；govulncheck 因网络无法跑完 |
| 仓库卫生 | 3 | 297MB FFmpeg 二进制入库、历史膨胀到 124MB、`logs/tasks.json` 被跟踪 |
| CI/CD | 6.5 | 多平台矩阵 + Docker 多架构 + 安装器自动化，超出同类个人项目水平 |
| 可观测性 | 5 | 内存日志环 + 控制台输出够用，但无文件日志轮转、无指标 |

**综合：约 4.2 / 10。** 项目"能用"，但在默认配置下**不应该**被当作局域网服务部署；桌面单机自用风险可控。

---

## 二、体检范围与方法

- 通读全部 22 个 Go 源文件（约 6198 行）与前端 `static/index.html`（5447 行 / 约 226KB）的关键路径；
- 审查 `Dockerfile`、`docker-compose.yml`、`.github/workflows/release.yml`、`installer.iss`、`start*.bat`、`.gitignore`、`go.mod/go.sum`；
- 运行验证：
  - `go build ./...` —— 通过（本机 Go 1.26.3，Windows/amd64）；
  - `go vet ./...` —— 零告警；
  - `gofmt -l .` —— **14 个文件**未格式化；
  - `go test ./...` —— 所有包 `[no test files]`；
  - `govulncheck` —— 尝试运行，因 `proxy.golang.org` 网络不可达未能完成，改为人工核对关键依赖版本；
- Git 事实核查：34 个提交、12 个 tag（v1.1.0 → v1.3.3 及 stable-v1.3.3）、`git ls-files` 与 `.gitignore` 对照。

---

## 三、架构速写

```
┌─ 桌面形态（Windows）────────────────────────┐
│ server.exe ──生成──> ccnew-vdl-window.ps1   │
│    │                    └─ WebView2 (WPF)   │
│    └─ Gin HTTP :18000 <────加载 static/index.html
└──────────────────────────────────────────────┘
internal/
  bilibili/   单视频解析、下载、M4S清理、FFmpeg合并、WBI签名、6类合集URL解析
  douyin/     单视频5策略降级、合集(普通/短剧)、专用下载器
  download/   任务管理器：创建/执行/进度/JSON持久化
  config/     环境变量 > 用户目录config.json > 程序目录config.json > 默认
  models/     早期模型（部分已被 handler 层副本取代）
cmd/server/   main(路由/CORS/杀端口) + handlers(~1737行核心API) + 平台启动器
```

数据流：前端粘贴链接 → `/api/tasks`（或 preview）→ Manager 解析 → 平台 Parser 拿直链 → Downloader 分流下载（B站双流转临时 m4s → 剥头 → FFmpeg 合并）→ 进度回写任务表 → `logs/tasks.json` / `collections.json` 持久化。订阅检查器每分钟扫描到期合集并增量下载。

---

## 四、安全体检（重点章节）

### S1【高危】API 无任何鉴权，且默认监听所有网卡

- 证据：`cmd/server/main.go:115` `addr := fmt.Sprintf(":%s", cfg.Port)` 绑定 `0.0.0.0`；全部路由无中间件鉴权。
- 影响：同一局域网内任何人可以：
  - 创建下载任务占用你的磁盘与带宽；
  - `POST /api/download-dir` 在进程权限可及的任意位置创建目录（`handlers.go:189`）；
  - 删除任务及其本地文件（`DELETE /api/tasks/:id?deleteFile=true`）；
  - 触发静默更新安装（见 S4）。
- 建议：默认绑定 `127.0.0.1`，Docker 场景由端口映射解决对外暴露；增加可选 Token（`SettingsRequest.APIToken` 模型已经预留了字段却从未实现）。

### S2【高危】两个开放代理接口构成 SSRF

- 证据：`handlers.go:369 ProxyImage` 与 `handlers.go:414 ProxyFileDownload` 直接 `http.Get(c.Query("url"))`，无协议白名单、无私网地址过滤、无重定向校验。
- 影响：局域网攻击者可让服务器请求 `http://169.254.169.254/`（云元数据）、内网管理面板等，图片代理还会原样回传内容类型与响应体。
- 建议：仅允许 `https` + 域名后缀白名单（hdslb.com/bilivideo.com/douyincdn 等），解析后逐跳校验 IP 不落私网/链路本地段。

### S3【高危】存储型 XSS：远程可控的视频标题/作者未转义直接进 innerHTML

- 证据：`static/index.html` 中存在 `escapeHtml()`（5719 行）但几乎只用于日志（4933、5711 行）；而任务列表 `3765` 行 `${task.title}`、`${task.author}`、合集列表约 `4611` 行 `${col.title}`、`${col.author}`、合集视频约 `4760` 行 `${v.title}` 均为裸插值；封面 URL 也裸拼进 `<img src="${...}">`。
- 攻击面：B站/抖音标题完全由第三方用户控制，形如 `<img src=x onerror=fetch('/api/download-dir',{method:'POST',...})>` 的标题一旦被预览/入列即执行。在桌面 WebView2 里该页面与全部 API 同源，等于把 S1/S2/S4 全部能力交到攻击者脚本手里。
- 建议：所有动态文本统一走 `escapeHtml`；封面 URL 先 `new URL()` 校验再拼属性；长期方案改用 `textContent` 或前端框架。

### S4【高危】自动更新：无校验下载可执行文件并静默运行

- 证据：`handlers.go:1737 TriggerUpdate` 从 GitHub API 取最新 release 的 `ccnew-vdl-setup.exe`，直接下载到 `%TEMP%` 并 `exec.Command(setupPath, "/SILENT", "/NORESTART")`，之后 3 秒 `os.Exit(0)`。全程没有 SHA256 校验、没有签名验证、甚至没有版本号比较（同版本也会重装）。结合 S1，局域网任意客户端都能触发。
- 建议：release 同时发布 `SHA256SUMS`，下载后哈希比对再执行；比较语义化版本仅在更新时动作；该接口必须纳入鉴权。

### S5【中危】CORS 前缀匹配放行畸形来源 + 无 Host 校验（DNS Rebinding 友好）

- 证据：`main.go:158-166` 用 `strings.HasPrefix(origin, "http://127.0.0.1")` 判定允许，`http://127.0.0.1.evil.com` 会命中；同时 `Access-Control-Allow-Credentials: true`。服务端不校验 `Host` 头，DNS rebinding 页面可将域名解析到 127.0.0.1 后以"同源"身份读取全部 API。
- 建议：Origin 精确匹配集合 `{http://127.0.0.1:PORT, http://localhost:PORT}`；生产模式直接拒绝带 Origin 的跨源请求；对 Host 做 `127.0.0.1:PORT` 白名单。

### S6【中危】Cookie 明文落盘与临时文件残留

- 证据：
  - `config/config.go Save()` 把含 Cookie 的完整配置写到 `~/.config/ccnew-vdl/config.json`，权限 0644；
  - `handlers.go LoginBilibili` 扫码登录后将 Cookie 写入 `%TEMP%\bilibili_cookies.txt`，读取成功后**从不删除**；
  - `docker-compose.yml` 将该配置目录挂载到宿主机 `./config`，明文随卷持久化。
- 建议：配置文件权限收紧至 0600（Windows 下 ACL 收敛）；登录临时文件用完即删；考虑用 OS 凭据库（DPAPI / Credential Manager）存 Cookie。

### S7【低危】killPortProcess 会强杀目标端口上的任意进程

- 证据：`main.go:24-40` 用 `netstat` 找监听进程后 `taskkill /F /PID`。如果用户把 PORT 配成常用端口（如 8080 上跑着别的服务），启动时会误杀无辜进程。
- 建议：只在确认 PID 属于自身旧实例时杀（PID 文件 + 进程名/路径校验），否则报错退出。

### 其他安全备注

- `sanitizeCookie` 清理 CR/LF/TAB 是对的（防头注入），值得肯定；
- `BrowseFolder`、`ToggleConsoleWindow`、控制台显隐等接口同样无鉴权，属 S1 的子项；
- 第三方解析兜底 `api.douyin.wtf` 会把用户访问的链接发给第三方服务，隐私上应至少做成默认关闭的开关。

---

## 五、功能性缺陷

### F1【严重】B站 WBI 签名算法实现错误

- 证据：`wbi_signer.go:87-126` 的 `Sign()` 对原始参数排序拼接后取 MD5，然后才把 `w_rid` 和 `wts` 作为返回值塞回参数集（`collection_parser.go` 中 `for k,v := range signParams { params[k]=v }`）。而 B 站官方算法要求先把 `wts` 并入参数、排序、过滤后再参与 MD5。
- 后果：空间列表类合集（`space.bilibili.com/{uid}/lists/{sid}`）走签名的三个 API 大概率被拒，实际靠"HTML 兜底爬取"续命——这解释了为什么该路径脆弱且慢。
- 建议：`Sign` 内部先 `params["wts"]=now` 再排序序列化，最后输出完整 query；补一个固定 key 的单测即可锁死正确性。

### F2【严重】番剧合集生成的 `ep` 链接无法下载

- 证据：`collection_parser.go:252` 把剧集 `BVID` 设成 `"ep"+id`；下载时 `handlers.go` 拼 `https://www.bilibili.com/video/ep12345`，而 `parser.go extractBVID` 只认 `BV[a-zA-Z0-9]+` 正则，必然匹配失败。
- 后果：**番剧 Season/Media 两类合集能预览、不能下载**，属于主流程断裂。
- 建议：番剧走 PGV playurl（`/pgc/player/web/playurl?episode_id=`），或在 VideoInfo 里直接携带 aid/cid 绕过 BVID 提取。

### F3【高】多P 视频展开后每一页都会下成 P1

- 证据：`parseSeriesCollection` / `tryAPIEndpoint` 把 `archive.Pages` 展开为多条记录（`Title - P%d`），但下载侧只拿 `BVID` 拼 URL，不带 `?p=N`，`getVideoInfoByBVID` 也永远返回第一 P 的 cid。
- 后果：N P 视频 × N 条记录 = 同一内容重复下载 N 份、文件名不同。
- 建议：合集条目保留 cid，下载前优先用 `aid+cid` 调 playurl；至少给 URL 追加 `?p=` 参数并在解析层尊重它。

### F4【高】抖音合集最多只拉 20 条

- 证据：`douyin/collection.go:160,161,333` 三处 API 均 `cursor=0&count=20` 且无翻页循环；HTML 兜底也只抓首屏 DOM。
- 后果：超过 20 个视频的合集/短剧被静默截断，`total_count` 与实际不符，订阅增量判断也会漏。
- 建议：按返回的 `has_more/cursor` 循环翻页直到收齐。

### F5【中】宣传的"CDN备用节点切换"实际是死代码

- 证据：`bilibili/downloader.go:199 DownloadWithMergeURLs`（候选 URL + 重试 + 节点切换的完整实现）在全仓库**没有任何调用方**；真实路径 `download/manager.go:493` 和 `handlers.go:996` 都调用的是只吃单一主 URL 的 `DownloadWithMerge`。Parser 辛苦构造的 `VideoURLs/AudioURLs` 备选列表在主流水线上被丢弃。
- 建议：Manager/Handlers 切换到 `DownloadWithMergeURLs`，或者删掉死代码避免误导。

### F6【中】合并下载进度会"倒车"

- 证据：`DownloadWithMerge` 先用 progressFunc 报视频流进度（0→100），随后音频流再次从 0 开始上报；UI 直接渲染该百分比。
- 建议：回调外层做权重合成（如视频 80% + 音频 20%）。

### F7【中】HTTP 探测逻辑存在崩溃边界

- 证据：`bilibili/downloader.go:66`、`douyin/downloader.go:394`：`io.ReadFull` 错误被忽略后执行 `string(buf[:n])[:5]`，当服务器在非视频 Content-Type 下返回不足 5 字节时直接 panic（Gin Recovery 能兜住请求线程，但下载 goroutine 会崩出任务栈）。
- 建议：`if n >= 5 && ...`，并用 `bytes.HasPrefix` 替代字符串切片。

### F8【中】删除任务不会停止进行中的下载（孤儿写入）

- 证据：`DeleteTask` 仅从 map 移除记录；`ExecuteTask` 的 goroutine 持有旧指针继续写 `outputPath`，进度回写在不存在 ID 上变成无效但下载本身继续消耗带宽，完成后还会在磁盘留下文件。
- 建议：Task 增加 `context.Context` 取消句柄，Delete 时 cancel；下载循环感知 ctx 中断。

### F9【中】并发上限会被合集数量放大，订阅刷新无节流

- 证据：每个合集各自 `sem := make(chan struct{}, 5)`；`refreshCollection` 对新增视频逐个 `go downloadSingleCollectionVideo(...)` 连信号量都没有。订阅 10 个合集各来 30 个新视频 = 数百并发协程同时抢网络与磁盘。
- 建议：Manager 级全局 semaphore（如 3-5），订阅批量刷新走队列。

### F10【中】持久化写入非原子，崩溃可能损坏数据

- 证据：`saveTasks` / `saveCollections` 直接 `os.WriteFile` 覆盖 JSON；`loadTasks` 解析失败时静默吞掉（历史清零）。
- 建议：写临时文件 + `os.Rename` 原子替换；load 失败时把坏文件改名留存 `.bak` 便于排查。

### F11【低】去重与体验细节

- 相同 URL 不同画质会命中 `FindTaskByURL` 直接返回旧任务（`CreateTask`）；短链未先 resolve 就参与去重，同一视频经短链/长链会建两个任务。
- `RetryTask`（handlers.go:718）在锁外改 `task.Status`，与 F12 的竞态问题叠加。

---

## 六、可靠性与并发

### R1【高】共享 Task 指针贯穿读写两端

- `GetTask` / `GetAllTasks` 返回内部 map 的 `*Task` 原指针（manager.go:285,289），Handler 序列化 JSON 的同时下载 goroutine 在锁内更新同一对象的 Progress/Speed——这是教科书级 data race（`go test -race` 下必现）。`RetryTask` 还有一处锁外赋值。
- 建议：对外一律返回值拷贝或快照结构；内部写操作全部走 `updateTask(func(t *Task))` 式闭包。

### R2【中】临时文件写在进程 CWD 且清理不保证

- 证据：`downloader.go:145` `temp_video_%d.m4s` 是相对路径；仓库根目录躺着一个 **438MB 的 `temp_video_1782647165709354300.m4s`** 就是历史崩溃/中断留下的实证。Docker 里则会污染 `/app`。
- 建议：临时文件放 `os.MkdirTemp` 或 `cfg.LogDir/tmp`；启动时清扫 `temp_*` 残留；下载成功路径上确保 defer 生效（当前正常路径没问题，异常退出靠启动清扫兜底）。

### R3【低】磁盘放大

- `removeM4SHeader` 为每条流额外复制一份完整文件（video.clean.mp4 + audio.clean.m4a），4K 视频峰值磁盘占用约为成品体积的 3 倍。
- 建议：用 `io.Copy` 时顺带 seek 跳头直接从原文件读（`input.Seek(8,0)` 即已如此），其实当前实现已是流式复制，真正的浪费是"clean 中间文件 + 最终合并"三层；可用命名管道（stdin）喂 FFmpeg 免掉中间文件。

### R4【低】优雅关闭不等下载

- `srv.Shutdown(ctx)` 只管 HTTP；正在进行的下载 goroutine 随进程退出硬断，配合 F10 会留下半截 JSON 与半截 mp4。

---

## 七、工程化与仓库卫生

### H1 二进制资产入库导致仓库肥胖

- `ffmpeg/` 目录 **297.6 MB**（windows/linux-amd64/linux-arm64/darwin-amd64 四份静态二进制）全部被 git 跟踪；`.git` 已膨胀到 **123.9 MB**（历史上还塞过更多）。darwin 版在 CI 中根本不构建对应产物，纯死重。
- WebView2 三个 DLL（0.8MB）入库尚可接受。
- 建议：FFmpeg 改为 CI 按需下载（workflow 里本来就有现成的下载逻辑注释史）；若坚持入库，用 Git LFS 并删掉 darwin 死资产；对历史做一次 filter-repo 瘦身（需协调团队）。

### H2 `logs/tasks.json` 被 git 跟踪

- `.gitignore` 明确写了忽略 `logs/tasks.json`，但它已在索引里（当前内容为空数组，暂无泄露）。这种"忽略但不生效"状态迟早会把真实浏览/下载历史提交上去。
- 建议：`git rm --cached logs/tasks.json` 一次性移出索引。

### H3 版本号三处漂移

- `installer.iss:4` 写死 `1.2.7`；`Dockerfile ARG VERSION=v1.3.2` 只是兜底默认；tag 已到 `v1.3.3`。HANDOVER 里说的端口不一致（19000/18000）代码侧已统一为 18000，但文档没同步。
- 建议：安装器版本由 CI 从 tag 注入（Inno 支持 `/D` 定义或预处理替换），Dockerfile 默认值仅作 dev 占位。

### H4 README 存在幽灵内容

- `POST /api/parse` 接口并不存在（实际是 `/api/tasks` 带 `quality:"preview"`）；`go build -o desktop ./cmd/desktop/` 目录不存在（桌面版就是 server.exe + PowerShell 启动器）。
- 建议：按现有路由表重写 API 章节；补一节"三种部署形态的真实启动方式"。

### H5 格式化与代码组织

- `gofmt -l` 列出 14 个文件（几乎覆盖所有核心包），说明提交前没有格式化门禁；
- `cmd/server/handlers.go` 单文件约 53KB / 1737 行，承担模型定义、20+ 个 Handler、订阅调度、持久化、更新器六种职责；
- 已知重复：`CollectionInfo/CollectionVideoInfo/LogEntry` 在 `models` 与 `handlers` 各一份；`identifyPlatform` 在 manager 与 handler 各一套判断逻辑；`douyin.Parser.Parse` 与 `DouyinDownloader.Parse` 五策略逻辑高度重叠。
- 建议：CI 加 `gofmt -l` 检查；handler 拆为 `task_handler/collection_handler/system_handler` + `service` 层；models 只留唯一真身，其余删除。

### H6 前端单体

- `index.html` 5447 行内联 CSS/JS，无构建、无模块、无转义规范（见 S3）。当前规模尚可维护，但每次改动都是高风险手术。
- 建议：短期先建立 `escapeHtml` 强制使用 + ESLint（no-unsanitized 插件）；中期拆出 `app.js/api.js/render.js` 三模块，仍可保持零构建。

### H7 依赖健康度

- `gin v1.9.1` 本体无已知未修 CVE（1.9.0 的 Content-Disposition 问题在 1.9.1 已修）；
- 但间接依赖明显陈旧：`golang.org/x/net v0.10.0`（其后修复了 HTTP/2 Rapid Reset CVE-2023-39325、CONTINUATION Flood、html 解析死循环等多个安全问题）、`golang.org/x/crypto v0.9.0`（Terrapin GO-2024-3321 修复于 v0.31.0）、`protobuf v1.30.0` 尚可。
- 说明：多数间接依赖未必进入运行路径，但供应链扫描器会持续红灯，升级成本低收益高。
- `govulncheck` 本次因代理网络不可达未能机器验证，建议网络可达时复跑一次。

### H8 Docker 与运行时

- 最终镜像基于 `alpine:latest`（未钉 digest），**容器以 root 运行**（无 USER 指令），无 HEALTHCHECK；构建阶段 `golang:1.22-alpine` 与 go.mod 声明一致但已是老工具链。
- 建议：钉 digest、`USER nonroot:nonroot`、加 HEALTHCHECK 打 `/api/stats`；docker-compose 的 `image: ...:latest` 至少换成具体版本 tag。

### H9 许可证合规提示

- 入库分发 FFmpeg 二进制：若这些 build 含 GPL 组件（常见 full build 均为 GPL），分发时需遵守相应开源义务（提供来源/许可文本）。当前仓库未见 FFmpeg LICENSE 附带。
- 建议：改用 LGPL 配置的自编译 FFmpeg 或在发行包/README 附带对应许可证全文。

---

## 八、做得好的地方（避免只挨打）

1. **B站 DASH 处理链路正确**：fnval=16 拿 DASH、剥 8 字节私有头、FFmpeg `-c copy` 合并，这套流程与社区实践一致（`downloader.go removeM4SHeader` 的"全零才剥"防御也稳）；
2. **抖音五策略降级设计务实**：分享页移动 UA → 桌面 RENDER_DATA → Detail API(ttwid) → 多 UA → 第三方 API，反爬对抗里正确的姿势；
3. **配置三级覆盖 + Cookie 消毒**（环境变量 > 用户目录 > 程序目录；CR/LF 清洗防头注入）；
4. **重启恢复语义明确**：parsing/downloading 状态重启后置为 failed 并注明原因；
5. **CI 超出同类项目水准**：三平台矩阵构建 + Inno 安装器 + Docker buildx 双架构 + DockerHub secret 缺失时优雅跳过；
6. **`go vet` 零告警**，说明基本静态卫生在线；
7. 合集订阅的"对比已有 URL 增量下载"思路简单有效（虽然并发治理欠账，见 F9）。

---

## 九、修复路线图（按优先级）

**P0（本周就该做，安全止血）**

1. 默认监听 `127.0.0.1`；可选 `AUTH_TOKEN` 中间件保护全部 `/api/*`；
2. 两个 proxy 接口加 https + 域名白名单 + 私网 IP 黑名单；
3. 前端所有 `${}` 动态文本过 `escapeHtml`（一次性 grep 清理约 46 处 innerHTML 插值点）；
4. 更新流程加 SHA256 校验 + 版本比较 + 鉴权；
5. 修 WBI 签名（wts 入参）并为 `Sign()` 补黄金样本测试；
6. 临时文件迁到专用 tmp 目录，启动清扫残留（顺手清掉根目录那个 438MB 文件）。

**P1（两周内，功能与可靠性）**

7. 修番剧 ep 下载（PGC playurl 直连 aid/cid）与多P `?p=` 透传；
8. 抖音合集翻页拉满；
9. Manager 返回任务快照消除 data race；Delete/Retry 接 context 取消；
10. 持久化改原子写（tmp+rename），坏文件备份；
11. 全局下载并发闸 + 订阅批刷走同一队列；
12. `git rm --cached logs/tasks.json`；FFmpeg 迁出 git（CI 下载或 LFS）。

**P2（一个月内，工程化提升）**

13. 引入测试基座：WBI 签名、文件名清洗、URL 识别、平台判定、M4S 剥头（构造字节切片即可测）、两个 proxy 的 SSRF 防护回归；CI 加 `gofmt -l && go vet && go test -race`；
14. handlers.go 拆分 + models 去重 + 删除 `DownloadWithMergeURLs` 死代码或正式启用；
15. 版本号单一来源（ldflags 注入 tag，installer/Docker 同源）；README 按真实路由重写；
16. Dockerfile 钉 digest + 非 root + HEALTHCHECK；依赖升级 x/net ≥0.33、x/crypto ≥0.31 后复跑 govulncheck；
17. FFmpeg 许可证合规（LGPL 构建或附 LICENSE）；决定 macOS 是补 CI 还是删资产。

---


---

## 十一、实战运营约束与开发流程规范

> 本节记录项目在实际运营和迭代过程中遇到的限制、踩坑经验和开发者强制约束的工作流程。
> 这些内容是体检报告通用建议之外的项目特有约束，修改代码前**必须**阅读并遵守。

### O1【强制】开发与发布工作流

```
┌─────────────────────────────────────────────────────────┐
│  修改源码 (D:\coderom\CCNEW-VideoDownloader)              │
│       ↓ go build -ldflags "-H=windowsgui"                │
│  编译 → 复制到 F:\迅雷云盘\抖音B站视频解析工具\            │
│       ↓ 用户测试确认                                      │
│  用户说"OK" → 才允许 git push + 打 tag 触发 CI             │
│       ↓ CI 自动构建                                       │
│  Windows安装包 + Linux二进制 + Docker镜像 → 自动发布       │
└─────────────────────────────────────────────────────────┘
```

**铁律：**
1. **绝不在用户确认前推送到 GitHub** — 用户多次因此发火，这是最高优先级约束
2. 测试统一使用 `F:\迅雷云盘\抖音B站视频解析工具\CCNEW-VideoDownloader.exe`
3. 桌面快捷方式指向 `F:\迅雷云盘\` 目录，不是 `D:\coderom\` 源码目录
4. 每次修改后必须 `go vet ./...` 通过才能复制到测试目录
5. 版本号通过 `ldflags -X main.Version=<tag>` 构建时注入，**不允许**在代码里硬编码版本字符串

### O2【已解决】抖音反爬对抗记录

**时间线：**
- v1.3.0 之前：RENDER_DATA + Detail API + HTML 正则三策略均正常
- 2026-08-16：抖音加强反爬，三策略全部失效
  - 分享页 `RENDER_DATA` 消失，替换为空的 `_SSR_DATA`（`data:{}`）
  - Detail API 返回空 body（需要 X-Bogus/a_bogus 签名）
  - `douyin.com/video/` 页面返回 JS 虚拟机反爬页面

**修复方案（v1.3.1）：**
- 新增 `getTtwid()` 函数，从 `https://ttwid.bytedance.com/ttwid/union/register/` POST 获取 `ttwid` cookie
- Detail API 加上 `ttwid` cookie 后恢复正常返回完整视频数据
- 策略顺序调整为 Detail API+ttwid 优先

**风险提示：** 抖音随时可能再次加强反爬。如果 Detail API 也开始要求 X-Bogus 签名，当前方案会再次失效。届时需要考虑：
- 实现 X-Bogus 签名算法（复杂度高，可能随时更新）
- 使用无头浏览器（Playwright/rod）渲染页面
- 接入第三方解析 API

### O3【已解决】B站 CDN 节点切换

**问题：** `xy42x56x84x181xy.mcdn.bilivideo.cn:8082` 等边缘节点连接被拒（`connectex: No connection could be made`），重试同一 URL 无效。

**修复方案：**
- Parser 解析 playurl 时同时收集 `backup_url` 和生成的 CDN 备选列表
- `cdnFallbackURLs()` 生成备选：去端口443、去端口、替换为通用镜像主机（`upos-sz-mirrorcos` 等）
- Downloader `downloadStreamWithFallback()` 按候选列表逐个尝试

**已知限制：** `DownloadWithMergeURLs`（候选列表版本）在 `download/manager.go` 的主流水线中**未被调用**，仍使用旧的 `DownloadWithMerge`（单一URL）。合集下载路径（`handlers.go`）已接入候选列表。此为体检报告 F5 所述的死代码问题，但因用户明确要求"不动下载合并逻辑"，暂不修改主流程。

### O4【已解决】版本号管理与 Docker 数据持久化

**问题1：版本号不一致**
- 代码中硬编码 `"v1.3.0"`，打 tag `v1.3.2` 后 Docker/Windows 仍显示旧版本
- 用户从 GitHub 下载 v1.3.2 安装包，装完显示 v1.3.1

**修复：** `main.go` 添加 `var Version = "dev"`，CI 通过 `ldflags -X main.Version=${{ github.ref_name }}` 注入，Dockerfile 通过 `ARG VERSION` 传递。

**问题2：Docker 更新丢数据**
- `docker-compose.yml` 未挂载 `logs/` 目录，`tasks.json` 和 `collections.json` 存在容器内部
- 用户用 Docker Copilot 拉新镜像重建容器后，下载记录全部消失

**修复：** `docker-compose.yml` 添加 `./logs:/logs` 卷挂载。

### O5【已解决】用户体验修复记录

| 问题 | 修复 | 版本 |
|---|---|---|
| 命令提示符窗口闪现 | `hideConsoleWindow()` + `-H=windowsgui` 编译 | v1.3.1 |
| 文件夹选择对话框不置顶 | TopMost Form 作为 Owner | v1.3.0 |
| 全局下载速度不显示 | header ⚡ 标签 + `task.Speed` 追踪 + `/api/stats` 返回 `global_speed` | v1.3.0 |
| 合集下载无速度 | `collectionSpeed` 字段 + 回调计算 | v1.3.0 |
| 合集订阅状态不明显 | 紫色「订阅中」徽章替代绿色「完成」 | v1.3.0 |
| 日志文字重叠 | `log-task` min/max-width + `word-break` + `line-height` | v1.3.0 |
| 操作日志格式混乱 | `op-log-item` 改 flex 布局 | v1.3.0 |
| B站 `actualUrl is not defined` | 前端变量作用域修复 | v1.3.0 |
| Cookie 含换行导致 API 报错 | `sanitizeCookie()` 清理 CR/LF/TAB | v1.3.1 |
| Docker 无 WebView2 扫码不可用 | 非 Windows 隐藏扫码按钮 | v1.3.1 |
| Windows 更新跳转 GitHub | `/api/update` 下载 setup.exe 静默安装 | v1.3.2 |

### O6【用户强制约束——不允许修改的部分】

以下内容用户明确要求**不得修改**，修改前必须征得同意：

1. **`MergeMP4` / `removeM4SHeader`** — B站视频合并逻辑是"命脉"，任何改动可能导致输出文件损坏
2. **下载合并主流程（`DownloadWithMerge`）** — 用户明确说"你可千万别动下载合并逻辑"
3. **已有的下载任务数据格式** — `tasks.json` / `collections.json` 的 JSON 结构变更需要迁移方案
4. **前端整体布局** — 用户对当前 UI 满意，不做大规模重构

### O7【容灾备份】

- **本地 Git 标签：** `stable-v1.3.3`（不推送远程）
- **完整源码备份：** `D:\CCNEW-VideoDownloader-backup-v1.3.3\`（47 文件，含 ffmpeg 全平台，634.5 MB）
- **编译好的 exe：** `CCNEW-VideoDownloader-stable-v1.3.3.exe`

恢复方式：将备份目录覆盖到工作目录，`go build` 重新编译即可。

### O8【Docker 部署约束】

**docker-compose.yml 标准模板：**
```yaml
services:
  video-downloader:
    image: wsc768043912/ccnew-vdl:latest
    container_name: video-downloader
    ports:
      - "18000:18000"    # 左边端口可改，右边18000不要动
    volumes:
      - ./downloads:/downloads                              # 下载目录，改成你的实际路径
      - ./config:/root/.config/ccnew-vdl                    # 配置文件，一般不用改
      - ./logs:/logs                                        # 任务数据，必须挂载否则更新丢数据
    environment:
      - PORT=18000
      - DOWNLOAD_DIR=/downloads
    restart: unless-stopped
```

**注意事项：**
- Cookie 在 Web 设置页面填写，不通过环境变量（避免特殊字符问题）
- Docker 环境无 WebView2，扫码登录按钮自动隐藏
- 更新镜像后必须 `docker compose down && docker compose up -d` 重建容器
- `./logs:/logs` 挂载是**必须的**，否则每次更新容器数据全部丢失

### O9【CI/CD 约束】

**GitHub Actions secrets：**
- `DOCKERHUB_USERNAME`: `wsc768043912`
- `DOCKERHUB_TOKEN`: DockerHub Personal Access Token (Read & Write)

**构建产物：**
| 文件 | 说明 |
|---|---|
| `ccnew-vdl-setup.exe` | Windows Inno Setup 安装包 |
| `ccnew-vdl-windows-amd64.zip` | Windows 便携版 |
| `ccnew-vdl-linux-amd64.tar.gz` | Linux x86_64 |
| `ccnew-vdl-linux-arm64.tar.gz` | Linux ARM64 (NAS/树莓派) |
| Docker `latest` + 版本 tag | 多架构镜像 (amd64 + arm64) |

**版本注入链：**
```
git tag v1.3.3
  → CI matrix: go build -ldflags "-X main.Version=v1.3.3"
  → Docker: ARG VERSION=v1.3.3 → go build -ldflags "-X main.Version=v1.3.3"
  → Installer: 需要手动同步 installer.iss 版本号（当前硬编码，待修复）
```

### O10【已知未修复问题清单】

以下问题在体检报告中有详细描述，但因优先级、风险或用户决策暂未修复：

| 体检编号 | 问题 | 未修原因 |
|---|---|---|
| S1 | API 无鉴权监听全网卡 | 个人桌面使用暂无暴露风险 |
| S2 | SSRF 代理 | 同上 |
| S3 | 存储 XSS | 前端改动风险大，暂缓 |
| F1 | WBI 签名错误 | 空间列表合集使用频率低 |
| F2 | 番剧 ep 无法下载 | 需要实现 PGC playurl，工作量大 |
| F3 | 多P 重复下载 | 需要改合集数据结构 |
| F5 | CDN 备选死代码 | 用户要求不动下载逻辑 |
| H1 | FFmpeg 297MB 入库 | 用户要求"不要管大小" |
| H3 | installer.iss 版本硬编码 | 待下个版本修复 |

---

## 十二、快速排障手册

### 抖音解析失败
```
1. 检查日志是否有 "[抖音] 策略1(parseDetailAPI+ttwid)成功"
2. 如果失败，检查 ttwid 获取是否正常（网络是否能访问 ttwid.bytedance.com）
3. 如果所有策略失败，抖音可能再次加强反爬，参考 O2 节的替代方案
```

### B站下载失败（CDN 连接被拒）
```
1. 检查日志是否有 "[B站-CDN] 切换到备用节点"
2. 如果合集下载仍失败，检查 handlers.go 是否使用了 DownloadWithMergeURLs
3. 单视频下载走 manager.go 的 DownloadWithMerge（无备选），这是已知限制
```

### Docker 更新后数据丢失
```
1. 检查 docker-compose.yml 是否有 ./logs:/logs 挂载
2. 如果没有，添加后 docker compose down && docker compose up -d
3. 已丢失的数据无法恢复
```

### 版本号显示不正确
```
1. 检查是否通过 ldflags 注入：go build -ldflags "-X main.Version=v1.3.3"
2. 检查 CI 是否传递了 github.ref_name
3. 检查 Dockerfile ARG VERSION 是否传递给 go build
```

### Windows 更新按钮不出现/跳转 GitHub
```
1. 检查 /api/config 返回的 platform 字段是否为 "windows"
2. 检查前端 checkForUpdate 中 isWin 变量是否正确赋值
3. 确认编译时使用了 -H=windowsgui（否则 runtime.GOOS 可能异常）
```

---

## 十三、验证记录附录

| 检查 | 命令 | 结果 |
|---|---|---|
| 编译 | `go build ./...` | 通过（Go 1.26.3 windows/amd64） |
| 静态分析 | `go vet ./...` | 0 告警 |
| 格式化 | `gofmt -l .` | 14 个文件不合格 |
| 测试 | `go test ./...` | 全部包无测试文件 |
| 依赖漏洞 | `govulncheck ./...` | 网络不可达未完成，人工核对替代 |
| Git 状态 | `git status --short --branch` | 干净，main 与 origin 同步 |
| 仓库体积 | PowerShell 统计 | ffmpeg/ 297.6MB；.git 123.9MB |

> 报告中的行号以本次体检时的 `7ac1d87` 为基准。

---

## 十四、对照第十一章开发逻辑后的复核结论

开发者补充的第十一章（运营约束）与本报告原建议逐条对照后，修订如下：

**撤回的建议（与用户明确决策冲突）**

- H1 全部处置建议（FFmpeg 迁出仓库、LFS、darwin 资产清理）——用户已明确"不要管大小"，构建确定性优先，不再作为建议项。
- R3 命名管道消除中间文件方案——需要改动 MergeMP4 调用链，落入 O6 冻结区，作废。
- F5 中"将主流水线切换到 DownloadWithMergeURLs"选项——即 O10 台账中已挂起项，且受 O6 下载主流程冻结约束；仅保留"若未来解冻再启用"的备注。

**表述修正**

- S1 的修复方向由"默认绑定 127.0.0.1"修正为：新增 HOST 环境变量分层——桌面模式默认回环，Docker 模式显式 0.0.0.0（容器内绑回环会使端口映射失效），暴露场景按需启用 Token。原写法与 O8 的 Docker 部署逻辑不兼容。
- F6 进度合成如实施，应放在 Manager 回调包装层而非 DownloadWithMerge 函数内部，以避开 O6 冻结区。

**对齐确认（维持挂起，无需行动）**

- S3/F1/F2/F3/H3 与 O10 台账记录一致，属于已知且有意识降级的事项，本报告不再重复催办。

**台账外提醒（O10 未收录、建议补充决策）**

1. ~~F4 抖音合集截断~~ → 根因已确认：普通合集无官方分享链接属平台限制而非代码缺陷；防御式翻页作为尽力而为的增强保留。真正的产品化方案是"贴主页→枚举合集→选择下载"，涉及前端改动，需用户点头后另行立项。
2. S1/S2 的豁免理由"个人桌面使用暂无暴露风险"仅覆盖桌面形态；O8 记录的标准 Docker/NAS 模板本身就是局域网暴露场景。建议在台账中把两种形态的风险接受范围分开声明，避免后续部署者误读。
3. S4（自动更新无校验）未出现在任何已接受清单中，仍是开放安全项。

**流程遵从声明**：本报告及后续任何代码修改均遵守 O1 工作流——改动先经用户在 F 盘测试目录确认，确认前绝不 push/打 tag。

---

**S2 修复进展（2026-08-25 更新）：已在本地完成 SSRF 跳板修复，待用户确认。**

- 改动范围：`cmd/server/handlers.go`（新增白名单闸门 + 双代理接口接入），不触碰 O6 冻结区。
- 三层防护：① 入口域名后缀白名单（仅 B站/抖音系媒体 CDN）+ 禁 IP 直连；② 重定向逐跳复验；③ 建连前校验解析 IP 拒绝私网/回环/链路本地段，防 DNS rebinding。
- 新增 `cmd/server/proxy_guard_test.go`：15 个用例全部通过（含伪装后缀、云元数据、内网 IP、file 协议等攻击样例）。
- 验证记录：`go vet` 零告警；`go build -ldflags "-H=windowsgui"` 编译通过；前端封面代理功能不受影响（hdslb/douyinpic/byteimg 均在白名单内）。

---

**第二批修复进展（2026-08-25 晚，用户授权"能修都修"后落地，均不触碰 O6 冻结区与 O10 挂起项）：**

| 编号 | 内容 | 状态 |
|---|---|---|
| F7 | 两个下载器 HTML 探测的 `buf[:n])[:5]` 越界 panic 改为 HasPrefix | 已修 |
| F10 | tasks.json / collections.json 改为临时文件+原子替换写入（新增 internal/fsutil）| 已修 |
| R1(部分) | RetryTask 锁外改状态改为锁内 ResetTaskForRetry | 已修 |
| S5 | CORS 从前缀匹配改为含端口精确匹配 | 已修 |
| S6a/b | config.json 权限 0644→0600；登录 Cookie 临时文件用完即删 | 已修 |
| S4 | 更新流程：版本相同短路 + SHA256 校验（校验文件缺失时降级告警放行）；CI 同步生成并上传 .sha256 | 已修 |
| H3 | installer.iss 版本改为可由 ISCC /DMyAppVersion 注入，默认回退 1.3.3；release.yml 已传参 | 已修 |
| F4 | 抖音普通合集翻页拉满（has_more/cursor 循环，上限500条）；短剧路径仍限20条待后续 | 已修(部分) |
| F4 | （经用户告知前任因接口不可翻页而損置）改为防御式翻页：首页失败照旧报错；第2页起任何失败/重复页均记日志并保留已有结果，最坏退化为旧版单页行为；上限500条。短剧路径维持单页 | 已修(防御式) |
| F4 | **根因确认（2026-08-26 用户发现）**：抖音普通合集没有官方分享链接，用户只能从浏览器地址栏拼 URL，入口先天缺失——这才是前任損置的真正原因，分页只是表层。现有防御式翻页保留：短剧（share/playlet 链接真实存在）及地址栏场景仍受益；产品化方案"贴主页→枚举合集→选择下载"涉及前端改动，需另行立项 | 根因已归档 |
| R2(部分) | 启动时清扫 CWD 内上次运行残留的 temp_*.m4s/clean 中间文件 | 已修 |
| S7 | killPortProcess 仅强杀同名旧实例，其他进程只告警不动手 | 已修 |
| H4 | README 移除幽灵 cmd/desktop 与幽灵 /api/parse，改为真实桌面启动方式与 preview 用法 | 已修 |

验证：go vet 零告警；全部测试通过（含新增 fsutil 原子写测试）；windowsgui 发布构建成功。F4/S4 的线上行为需在 F 盘测试环境实测确认。

**第三批：新版 UI 并行开发（2026-08-26，用户授权立项）**

- 架构：新增独立文件 `static/index-v2.html`（约 44KB），与老 UI 完全隔离；后端零改动。老 `index.html` 仅在页尾加一颗悬浮“新版界面”切换球（用户明确授权的唯一改动）。
- 切换机制：经典页右下角发光按钮 → 跳转 `/static/index-v2.html` 并记忆偏好；新版左下角“经典界面”随时切回并清除偏好；无死循环。
- 新版设计：深空石墨底 + 终端绿主色/青色辅色的下载控制台风；左侧图标导航（仪表盘/任务/合集/日志/设置）；仪表盘含全局速度均衡器动画、四项统计火花条、最近任务与系统事件双信息流；任务页解析坞+预览卡+进度条纹动画；合集卡片墙+详情弹窗（单视频操作/订阅开关）；终端风日志台（级别过滤/自动滚动）；分组设置面板（目录浏览/B站扫码/抖音 Cookie/控制台/更新）。
- 安全继承：全部动态文本经 esc() 转义（根除老版 S3 类 XSS 模式）；封面走白名单代理；图标内嵌 SVG 无 CDN 依赖。
- 验证：花括号配平、node --check JS 语法通过；F 盘实例实测首页/新版页均 200 且关键标记存在；stats API 正常。旧页面备份为 F 盘 `static/index.html.old`。
- **切换按钮可见性排查（2026-08-26）**：用户反馈看不到按钮。经无头浏览器截图 + 服务端字节对比 + 红色测试块二分法定位：按钮一直正常渲染，问题在于①原样式过于低调；②WebView2 持久化缓存导致窗口显示旧页面快照——桌面窗口只在启动时加载一次页面，后续文件更新不会自动刷新已开窗口。
- 处置：按钮改为实心绿底高亮样式；启动器 URL 加时间戳参数（每次启动强制新导航绕过文档缓存）；旧 WebView2 缓存目录改名让位；新 exe 已重建部署（含 launcher 改动）。最终截图确认绿按钮在顶栏清晰可见。
