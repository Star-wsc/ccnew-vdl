# DouBi 修复记录（Fix Log）

> 专记"遇到什么问题 → 根因是什么 → 怎么修的"。
> 每条记录含：症状、根因、修复、影响版本。按时间倒序排列。

---

## 2026-09-07

### B站音频流403下载失败
- **症状**：极客湾视频下载失败，日志报 `下载音频流失败(重试3次): HTTP 403`
- **根因**：B站CDN链接含签名会过期。`DownloadWithMerge`只接受单URL，重试3次都是同一个过期链接。解析器已拿到`backup_url`列表但`videoInfo`只存了第一个URL。
- **修复**：`videoInfo`加`VideoURLs`/`AudioURLs`候选列表字段；有候选时走`DownloadWithMergeURLs`（内置多URL自动切换+403重试）；preview路径重新解析也同步获取候选列表。
- **版本**：v1.4.18
- **文件**：`internal/download/manager.go`

### B站清晰度标签永远显示4K（虚标）
- **症状**：B站视频实际1080P/480P，外显一直标4K
- **根因**：三层叠加——
  1. `qualityName(qn)`只认120/80/64/32四个精确值，B站的112(1080P高码率)、116(1080P60帧)走default直接返回数字字符串
  2. `createFromPreview`链路里`task.Quality`直接存APP请求参数"4k"，`ExecuteTask`的preview路径跳过解析直接用
  3. `ParseVideo`对外方法用请求参数覆盖`actualQuality`而不是用解析器真实值
- **修复**：
  1. `qualityName`改为范围判断（≥120=4K / ≥80=1080P / ≥64=720P / ≥32=480P）
  2. `ExecuteTask` preview路径强制重新解析，用B站API真实`actualQn`覆盖
  3. `ParseVideo`返回`videoInfo.Quality`（解析器真实值）而非请求参数
- **版本**：v1.4.17 / v1.4.18
- **文件**：`internal/bilibili/parser.go`、`internal/download/manager.go`

### 抖音清晰度标签虚标（API gear_name含"4k"但实际流480P）
- **症状**：抖音视频实际480P，外显标4K
- **根因**：抖音API返回的`gear_name`字段常含"4k"字样，`mapQualityAdvanced`在宽高命中低档位后还会走gear_name回退匹配，把480P流标成4K
- **修复**：`mapQualityAdvanced`有实际宽高时禁用gear_name回退；下载后用`detectActualQuality`(ffprobe)后验修正
- **版本**：v1.4.16
- **文件**：`internal/douyin/parser.go`、`internal/download/manager.go`

### 清晰度ffprobe判断逻辑错误（1440×1080标成2K）
- **症状**：4:3比例的1080P视频（1440×1080）被标成2K
- **根因**：`detectActualQuality`取最大边（1440≥1440→2K），但1080P定义是短边1080
- **修复**：取较短边判断（`min(w,h)`）。1440×1080→短边1080→1080P ✓
- **版本**：v1.4.18
- **文件**：`internal/download/manager.go`

---

## 2026-09-06

### YouTube封面国内加载不出来
- **症状**：YouTube视频封面空白
- **根因**：封面域名`i.ytimg.com`（Google CDN）国内直连不可达。服务器的封面代理`/api/proxy/image`白名单里没有`ytimg.com`，且出站请求没走YouTube代理
- **修复**：`ytimg.com`/`ggpht.com`加入代理白名单；YouTube系资源出站自动走`yt_proxy`；走显式代理时跳过拨号IP校验（SSRF防护误杀代理地址）
- **版本**：v1.4.14
- **文件**：`cmd/server/handlers.go`

### APP YouTube封面加载失败
- **症状**：APP里YouTube视频封面空白（服务器代理已修但APP直连ytimg）
- **根因**：APP的`Image.network`直连ytimg域名，国内手机访问不到
- **修复**：`ApiService.coverSrc()`路由——YouTube系封面自动经服务器代理加载
- **版本**：v1.6.1
- **文件**：`android_flutter/lib/services/api_service.dart`

### B站b23.tv短链接报"无法提取BVID"
- **症状**：B站短链接`b23.tv/xxx`解析失败
- **根因**：`extractBVID`只做正则匹配`BV[字母数字]`，短链里没有BV字样。历史上从来没支持过（不是这次改坏的）
- **修复**：`ResolveShortURL`跟随302跳转拿真实URL（学抖音`v.douyin.com`的做法），在`Parse`和`ParseCollection`入口加短链预处理
- **版本**：v1.4.8
- **文件**：`internal/bilibili/parser.go`、`internal/bilibili/collection_parser.go`

### 任务数据丢失事故
- **症状**：8个APP任务记录消失
- **根因**：`saveTasks()`用非原子写入（`os.WriteFile`直接写），频繁`kill -9`重启时撞上写盘→文件截断→重启解析失败→空列表反写覆盖原文件
- **修复**：`saveTasks`改为原子写入（临时文件+`os.Rename`）；`loadTasks`失败记ERROR日志（之前静默失败）
- **教训**：部署时不要用`pgrep -f ccnew-vdl`（会匹配到SSH命令行自身），用`pkill -9 -x ccnew-vdl`
- **版本**：v1.4.15
- **文件**：`internal/download/manager.go`

### 任务卡片全部消失
- **症状**：Web控制台任务列表变空白
- **根因**：移动复选框时误删了`const checked=SEL.has(t.id)`这行，`renderTasks`抛ReferenceError导致整个列表渲染中断
- **修复**：加回`checked`变量声明
- **版本**：v1.4.15
- **文件**：`static/index-v2.html`

### APP下载视频没声音（抖音DASH音视频分离）
- **症状**：APP下载的抖音视频没有音频，Web端正常
- **根因**：`ParseVideo`返回的map漏了`audio_url`字段，`createFromPreview`创建的任务先天缺音频。Web端直接提交URL走完整解析能拿到audio_url，APP走preview链路拿不到
- **修复**：`ParseVideo`返回值补`audio_url`；`Task`加`AudioURL`字段；`create-from-preview`提取`preview_data.audio_url`
- **版本**：v1.4.6
- **文件**：`internal/download/manager.go`、`cmd/server/handlers.go`

### 合集创建后不下载
- **症状**：APP创建的合集一直挂在pending不动
- **根因**：APP创建合集时没传`auto_download`参数，服务器按"只创建不下载"处理
- **修复**：`createCollection`强制`auto_download=true`，quality为空时默认1080p
- **版本**：v1.4.7
- **文件**：`android_flutter/lib/services/api_service.dart`

### 合集状态不持久化（重启后回退pending）
- **症状**：服务器重启后已完成的合集状态丢失，文件路径清空
- **根因**：`downloadCollectionVideos`下载完成后从不调`saveCollections()`，状态只存在内存里
- **修复**：完成回调里加`saveCollections()`
- **版本**：v1.4.9
- **文件**：`cmd/server/handlers.go`

### APP保存到相册后无法播放
- **症状**：视频保存到系统相册后打开播放失败
- **根因**：`MediaStoreHelper`用8KB默认缓冲写入不完整 + `IS_PENDING`可能提前清除 + `file://`路径在高版本Android不可靠
- **修复**：64KB缓冲+写入字节数校验+返回`content://`URI用于播放（不再依赖临时文件路径）
- **版本**：v1.5.0
- **文件**：`MediaStoreHelper.kt`、`native_bridge.dart`、`player_page.dart`

### 设置B站Cookie后下载还是游客画质
- **症状**：通过`/api/bilibili/cookie`设置Cookie后，下载仍然没有高清流
- **根因**：`SetBilibiliCookie`端点只同步了`cfg`和`bilibiliParser`，漏了`bilibiliCollection`和`mgr`（下载链路），要重启服务器才生效
- **修复**：全量同步（parser+collection+mgr.SetBilibiliCookie）
- **版本**：v1.4.10
- **文件**：`cmd/server/handlers.go`

### B站BVID视频删不掉
- **症状**：合集内的B站视频点删除无反应
- **根因**：`DeleteCollectionVideo`只按抖音的`video_id`匹配，B站视频该字段为空（只有`bvid`）
- **修复**：兼容BVID匹配，同时删除本地文件
- **版本**：v1.4.9
- **文件**：`cmd/server/handlers.go`

---

## 2026-09-05

### APP任务和Web任务混在一起
- **症状**：手机上创建的任务在Web端也显示
- **根因**：无来源标记，所有任务共享一个列表
- **修复**：Task/Collection加`source`字段；APP创建时带`source=app`；查询时Web端自动排除`source=app`的单视频任务（合集保持共享）
- **版本**：v1.4.7
- **文件**：`internal/download/manager.go`、`cmd/server/handlers.go`、`api_service.dart`

### APP冷启动被踢到设置页
- **症状**：服务器连不上时APP直接跳设置页，缓存没机会展示
- **根因**：路由只看`checkConnection`结果，没考虑离线缓存
- **修复**：`_shouldGoHome`——配过服务器地址且手机有缓存就进主页（离线模式）
- **版本**：v1.5.0
- **文件**：`main.dart`

### 离线时界面被清空、缓存被覆盖
- **症状**：服务器挂了后APP界面变空白，重启后缓存也没了
- **根因**：`getStats`/`getTasks`吞网络错误返回空数组 → 界面被空数据覆盖 → 空数据还回写了本地缓存
- **修复**：轮询先`checkConnection`，失败时保持本地缓存内容不动
- **版本**：v1.5.0
- **文件**：`home_page.dart`

### 流量消耗大（每2秒三连击轮询）
- **症状**：APP后台也在疯狂请求服务器
- **根因**：无脑2秒轮询stats+tasks+collections，后台不停
- **修复**：事件驱动刷新——切菜单刷新（1分钟CD）、操作立即刷新（绕过CD）、常驻5分钟兜底、下载中2秒实时、后台零请求
- **版本**：v1.5.0
- **文件**：`home_page.dart`

### 日志模块崩溃
- **症状**：日志页显示JSON乱码或空白
- **根因**：`/api/logs`返回JSON数组，APP当纯文本渲染；日志拉取还被1分钟CD拦截
- **修复**：解析JSON格式化为`HH:MM:SS [LEVEL] message`行+分级着色；日志拉取移出CD块
- **版本**：v1.5.0
- **文件**：`api_service.dart`、`home_page.dart`

### 播放器"无法播放此视频"和"正在加载"叠显
- **症状**：加载中就显示错误提示
- **根因**：`!_initialized`同时渲染加载和错误文案
- **修复**：加`_failed`状态，只有真正失败才显示错误
- **版本**：v1.5.0
- **文件**：`player_page.dart`

---

## 2026-09-04（Android版开发阶段）

### Flutter构建报"repository maven冲突"
- **症状**：`flutter build apk`报flutter-plugin-loader解析失败
- **根因**：全局`~/.gradle/init.gradle`（阿里云镜像）与Flutter插件解析冲突
- **修复**：构建前改名`init.gradle.bak`，构建后恢复
- **文件**：构建流程

### NDK版本不匹配
- **症状**：插件要求更高NDK版本
- **根因**：path_provider/video_player/jni各要各的NDK版本
- **修复**：`build.gradle`统一`ndkVersion=28.2.13676358`
- **文件**：`android/app/build.gradle`

### 合集列表要切Tab才刷新
- **症状**：创建合集后合集页不显示，必须手动切过去
- **根因**：`_poll()`里合集只在`_tab==1`时拉取
- **修复**：合集进入全局轮询（每轮都拉）
- **版本**：v1.5.0
- **文件**：`home_page.dart`

### APP平台徽章日间模式看不清
- **症状**：抖音"D"徽章日间模式下黑底+深色字糊成一团
- **根因**：字母颜色用主题色`text1`（日间深色），背景黑色半透明
- **修复**：B/D字母固定白色，两种模式都清晰
- **版本**：v1.5.0
- **文件**：`home_page.dart`

### 设置页版本号硬编码
- **症状**：APP更新后设置页版本号不变
- **根因**：版本号写死`v1.4.0`，没有运行时读取
- **修复**：通过`PackageManager`运行时读取真实版本号
- **版本**：v1.5.0
- **文件**：`settings_page.dart`
