# DouBi (CCNEW-VideoDownloader) 开发文档

> 架构、环境、流程、问题解决手册。
> 红线与约定见 [CLAUDE.md](CLAUDE.md)，两者冲突时以 CLAUDE.md 为准。

---

## 一、架构总览

```
┌─ 手机 APP（Flutter 瘦客户端 v1.5.0+）─────────┐
│  UI(液态玻璃) / 相册保存 / 内置播放 / 更新检查      │
│  - LocalStore: 任务/合集/统计本地缓存，启动秒开     │
│  - 刷新: 事件驱动 + 1min CD + 5min兜底 + 下载2s实时 │
│  - 下载到相册: Dio流式下载 → MediaStore → content:// │
└──────────────┬───────────────────────────┘
               │ HTTP (source=app 隔离)
┌──────────────▼───────────────────────────┐
│ Ubuntu 服务器 <server-ip>:18000          │
│ Go + Gin (v1.4.x)，systemd: ccnew-vdl      │
│  - 抖音/B站解析(多策略+重试) 全部在此        │
│  - DASH 音视频下载 + ffmpeg 合并            │
│  - 合集管理/订阅刷新/移动端流式接口           │
│  - Web 控制台(DouBi 控制台) static/         │
└──────────────────────────────────────────┘
```

- 手机端**不做**任何解析/下载/合并（历史教训：Go 二进制在 Android 被 Seccomp 禁止
  fork/exec，FFmpeg so 有文本重定位问题 → 全部挪服务器，APP 只是壳）。
- 任务隔离：APP 创建的任务带 `source=app`，Web 端查询不带 source 看全部。

## 二、环境

| 项 | 值 |
|---|---|
| Flutter | `C:\flutter347\flutter`（3.47.2，Dart 3.8.1） |
| Android SDK | `C:\Users\wsc76\AppData\Local\Android\Sdk` |
| adb | `.../Sdk/platform-tools/adb.exe` |
| 服务器 | `<user>@<server-ip>`，目录 `/home/<user>/ccnew-vdl-dev/` |
| 服务器服务 | systemd `ccnew-vdl`，Restart=always，端口 18000 |
| 服务器日志 | `/home/<user>/ccnew-vdl-dev/logs/server.log`（GIN 请求日志在 journal） |
| 手机 | Xiaomi 17 Pro Max，无线调试（adb mdns 自动发现） |
| APK 产物 | `android_flutter/build/app/outputs/flutter-apk/app-debug.apk` → 桌面 `DouBi下载器.apk` |

## 三、关键代码地图

### 服务器 (Go)
| 文件 | 职责 |
|---|---|
| `cmd/server/main.go` | 路由注册、静态目录 |
| `cmd/server/handlers.go` | 全部 HTTP handler、合集管理、collections.json 持久化 |
| `internal/download/manager.go` | 任务管理器：parseVideo/downloadVideo/重试/输出路径 |
| `internal/bilibili/parser.go` | B站解析（含 b23.tv 短链 ResolveShortURL） |
| `internal/bilibili/downloader.go` | B站下载+合并（DownloadWithMerge） |
| `internal/douyin/downloader.go` | 抖音解析（5策略+enrichAudioURL） |
| `internal/douyin/parser.go` | 抖音 detail API、bit_rate_audio 音频流提取 |

### 移动端 (Flutter)
| 文件 | 职责 |
|---|---|
| `lib/services/api_service.dart` | 全部服务器 API（创建/查询/合集/cookie/preview） |
| `lib/services/local_store.dart` | 本地缓存（tasks/collections/stats.json，变化才写盘） |
| `lib/services/download_manager.dart` | Dio 从服务器下载到临时目录（带进度） |
| `lib/services/native_bridge.dart` | MethodChannel: 相册/安装APK/网络类型/版本 |
| `lib/services/update_service.dart` | GitHub Releases 检查更新（app-v tag）+ APK 下载 |
| `lib/pages/home_page.dart` | 主页三 Tab + 刷新调度(_refresh/_armStandingTimer) |
| `lib/pages/settings_page.dart` | 服务器/主题/下载设置/关于/检查更新 |
| `lib/pages/player_page.dart` | 播放器（directUrl > content:// > 服务器URL） |
| `lib/widgets/preview_dialog.dart` | 链接预览弹窗（单视频/合集选择） |
| `android/.../MainActivity.kt` | MethodChannel 实现 + 分享 Intent 接收 |
| `android/.../MediaStoreHelper.kt` | 相册写入（64KB缓冲+大小校验+返回URI） |

### 服务器关键接口
```
POST /api/tasks                       创建任务/预览(quality=preview)
POST /api/tasks/create-from-preview   从预览创建(带audio_url, source)
GET  /api/tasks?source=app            任务列表(隔离)
GET  /api/tasks/:id/download          下载已完成文件
GET  /api/stats?source=app            统计(含global_speed)
POST /api/collections                 创建合集(必须带auto_download=true)
GET  /api/collections/:id/videos/:idx/file  播放合集内视频
DELETE /api/collections/videos/:id    删合集内视频(VideoID或BVID匹配)
POST /api/collections/:id/subscribe   订阅开关
POST /api/settings                    cookie设置(全量同步mgr)
POST /api/bilibili/cookie             cookie设置(已修复为全量同步)
GET  /api/bilibili/cookie             查cookie状态(掩码)
```

## 四、开发流程

### 服务器迭代
1. 改代码 → 本地 `go build ./cmd/server/` 验证
2. 按 CLAUDE.md 版本规则确定新版本号
3. 交叉编译 + scp + kill/cp 部署（见 CLAUDE.md 第三节）
4. `curl /api/config` 验证版本号变化
5. 涉及静态页面：单独 `scp static/index-v2.html` 到服务器 `static/` 目录

### 移动端迭代
1. 改代码 → `flutter analyze lib/` 检查
2. pubspec 版本号 +1（versionCode 递增！）
3. init.gradle 改名 → `flutter build apk --debug --android-skip-build-dependency-validation` → 恢复
4. `adb install -r <apk>` 装机验证（可配合 adb input tap + screencap 走查 UI）
5. 拷贝到桌面 `DouBi下载器.apk`
6. git commit

### 发移动版（GitHub 更新检查用）
1. `git tag app-vX.Y.Z && git push origin app-vX.Y.Z`
2. GitHub Release 上传 APK 附件
3. 用户 APP 设置页即可检查到并下载安装

## 五、问题解决手册（历史踩坑）

### 架构类
| 问题 | 根因 | 解决 |
|---|---|---|
| Go 二进制在 Android 无法运行 | Seccomp 禁止 fork/exec | 弃用本地服务，改瘦客户端连远程服务器 |
| FFmpeg so 加载失败 | 文本重定位，动态链接器拒绝 | 合并全部在服务器执行 |
| Web端任务和APP混在一起 | 无来源标记 | Task/Collection 加 source 字段+查询过滤 |

### 解析/下载类
| 问题 | 根因 | 解决 |
|---|---|---|
| APP下载的视频没声音，Web端正常 | ParseVideo 返回 map 漏 audio_url，preview 创建的任务先天缺音频 | ParseVideo 补 audio_url；Task 加 AudioURL 字段；create-from-preview 提取 |
| B站 b23.tv 短链报"无法提取BVID" | extractBVID 只做正则，短链无 BV 字样（历史上就没支持过） | ResolveShortURL 跟随 302 Location 拿真实 URL（学抖音 v.douyin.com 的做法） |
| B站合集内视频删不掉 | DeleteCollectionVideo 只匹配 VideoID，B站视频该字段为空 | 兼容 BVID 匹配，顺带删本地文件 |
| 合集创建了但不下载 | APP 没传 auto_download，服务器按"只创建"处理 | createCollection 强制 auto_download=true + 默认画质 |
| 服务器重启后合集回 pending、文件路径丢 | downloadCollectionVideos 完成后没 saveCollections | 完成回调里持久化 |
| 设置 cookie 后下载还是游客画质 | /api/bilibili/cookie 只同步解析器不同步 mgr | 全量同步：parser+collection+mgr.SetBilibiliCookie |
| 下载画质不是最高 | APP 硬编码 1080p | 默认 4k，服务端挑返回流中最高 |

### 移动端类
| 问题 | 根因 | 解决 |
|---|---|---|
| 保存到相册后无法播放 | 8KB 默认缓冲写入不可靠 + file:// 路径不可靠 | 64KB 缓冲+写入字节数校验+返回 content:// URI 播放 |
| 播放失败时"正在加载"和"无法播放"叠显 | !_initialized 同时渲染两个文案 | 加 _failed 状态，只有失败才显示错误 |
| 合集列表要切 tab 才刷新 | _poll 里合集只在 _tab==1 时拉取 | 合集进入全局轮询；后演进为事件驱动 |
| 冷启动被踢到设置页（缓存没机会显示） | 路由只看 checkConnection 结果 | _shouldGoHome：有缓存就进主页（离线模式） |
| 离线时界面被清空、缓存被覆盖 | getStats/getTasks 吞网络错误返回空数组 | _refresh 先 checkConnection，失败不动本地数据 |
| 流量消耗大 | 无脑 2s 三连击轮询，后台也在跑 | 事件驱动+CD+5min兜底+下载2s实时+后台零请求 |
| 播放器打不开合集视频 | APP 拿 bvid 调 /api/tasks/:id/download（404） | 服务端新增 /api/collections/:id/videos/:idx/file |

### 构建/环境类
| 问题 | 根因 | 解决 |
|---|---|---|
| Gradle 报 "repository 'maven' was added by settings file" | 全局 ~/.gradle/init.gradle（阿里云镜像）与 Flutter 插件解析冲突 | 构建前改名 init.gradle.bak，构建后恢复（见 CLAUDE.md） |
| 插件要求更高 NDK | path_provider/video_player/jni 等各要各的 | build.gradle 统一 ndkVersion=28.2.13676358 |
| Flutter 3.32 装不了 liquid_glass_widgets | 包要求 Flutter 3.41+ | 升级 Flutter 3.47.2 |
| flutter build 无输出卡住 | tail 管道缓冲吞输出 | 直接跑不加管道，或 run_in_background 后读日志文件 |
| adb shell screencap 全黑 | 小米翻盖机双屏，默认截到副屏/熄屏 | `screencap -d <displayId>`；先 WAKEUP+KEYCODE 82 |
| adb 路径报 C:/Program Files/Git/data/... | Git Bash MSYS 路径转换 | `export MSYS_NO_PATHCONV=1` |
| cp 二进制报"文本文件忙" | 进程还在跑 | 先 kill（kill 和 cp 分两条 ssh，防止连接中断）；部署后 md5sum 比对确认 |
| systemctl 报需要交互认证 | ssh 无 sudo 密码 | kill 让 systemd(Restart=always) 5 秒自愈 |
| journalctl 无权限/无输出 | sudo 需密码；GIN 日志在 journal 不在 server.log | 用 `systemctl status ccnew-vdl -n 50 -o cat` 看最近请求 |

## 六、验证习惯（每次改完必做）

1. **服务器**：go build 通过 → 部署 → curl /api/config 看版本 → curl 相关接口看行为
2. **移动端**：flutter analyze → 装机 → adb 截屏走查关键页面（日间/夜间都要看）
3. **多主题检查**：任何 UI 改动，日间模式+夜间模式各截一次，重点看文字与背景对比度
4. **离线检查**：杀服务器进程 → APP 操作 → 界面应保持缓存内容且显示"未连接"
