# DouBi 开发限制与核心规则（红线文件）

> 本文件是项目最高约束。任何迭代、重构、修 bug 都必须遵守。
> 违反红线 = 返工。历史教训见 DEVELOPMENT.md 的问题手册。

---

## 一、红线（绝对禁止）

### 1. 不许动服务器核心逻辑
- `internal/douyin/`、`internal/bilibili/` 的**解析逻辑**、`mergeAudioVideo` 合并逻辑、
  下载重试策略属于核心。改核心必须先向用户说明并获得明确同意。
- 历史教训：改合并逻辑导致"所有视频没声音"，用户两次发火("你动核心了？")。
  排查问题先怀疑外围（接口返回、参数传递），核心代码 git 历史可证清白。

### 2. 版本号强制迭代
- **每次迭代必须更新版本号**，禁止"改了代码不改版本"。
- **服务器版与移动版独立迭代，版本号互不关联。**
- 服务器版：构建时 `-X main.Version=X.Y.Z` 注入，部署后必须 `curl /api/config` 验证版本变化。
- 移动版：`android_flutter/pubspec.yaml` 的 `version: X.Y.Z+N` 是唯一来源，
  N(versionCode) **每次发版必须递增**（Android 覆盖安装硬要求）。
- 用户可见的版本号必须运行时读取（PackageManager / /api/config），**禁止硬编码**。

### 3. 命名
- 用户可见处统一 **DouBi**（APP名、控制台标题、相册目录）。
- 仓库路径保留 CCNEW-VideoDownloader（历史原因），但新写的用户可见文案禁止出现 CCNEW。

### 4. 流量与电量
- 禁止任何形式的持续秒级轮询。数据刷新走事件驱动：
  - 列表以手机本地缓存为准（LocalStore）
  - 切菜单刷新当前菜单（受 1 分钟 CD 约束）
  - 页内操作（创建/删除/下载）立即刷新（绕过 CD）并重置 CD
  - 常驻兜底轮询 5 分钟一次，仅前台
  - 有任务解析/下载中 → 2 秒实时通道（绕过 CD）
  - APP 后台 → **零请求**
- 离线时保持缓存内容显示，**绝不允许用空数据覆盖界面或本地缓存**。

### 5. APP 与 Web 隔离
- APP 创建的一切任务/合集必须带 `source=app`，查询带 `?source=app`。
- Web 端不受 APP 影响，互不干扰（后端共用，前端独立）。

---

## 二、核心约定

### 下载
- APP 创建任务/合集默认清晰度 **4k**（服务端 B站 DASH 自动挑返回流中最高，
  无权限时 B站 API 自然降级；抖音始终取最高流）。
- B站下载必须使用**服务器配置的 cookie**（manager 启动加载 + 设置接口全量同步）。
  cookie 相关接口改动必须同步 `mgr.SetBilibiliCookie`，否则下载链路拿旧 cookie。

### 保存与播放
- 保存到相册：MediaStore 写入，成功后返回 `content://` URI 用于播放。
- 播放优先级：`directUrl`（合集视频）> content URI/本地文件 > 服务器 URL。
- 删除确认弹窗**必须包含**提示："已保存在相册内的视频不会被删除"。

### 更新检查
- 只认 GitHub Releases 中 tag 前缀 `app-v` 的 release（移动版专用线）。
- 发移动版：tag = `app-vX.Y.Z`，附件含 `.apk`。
- 服务器版不检查更新，手动部署。

### UI
- 所有文字颜色跟随主题变量（text1/text2/text3/tp.textPrimary 等），
  **禁止硬编码白色或深色**——日间夜间都要过一遍。
- 平台徽章 B/D 字母固定白色（黑底/粉底）。

---

## 三、构建与部署限制

### Flutter 构建前必做
```bash
# 1. 改名阿里云镜像配置（不改名必失败：flutter-plugin-loader 解析冲突）
mv ~/.gradle/init.gradle ~/.gradle/init.gradle.bak
# 2. 构建
flutter build apk --debug --android-skip-build-dependency-validation
# 3. 构建完恢复
mv ~/.gradle/init.gradle.bak ~/.gradle/init.gradle
```
- NDK 版本锁定 `28.2.13676358`（plugins 要求），改版本前先看插件要求。

### 服务器部署流程
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=X.Y.Z" -o server-linux-amd64 ./cmd/server/
scp server-linux-amd64 <user>@<server-ip>:/tmp/
# 先 kill 再 cp（直接 cp 会报"文本文件忙"），systemd Restart=always 5秒后自动拉起
ssh "kill -9 \$(pgrep -f ccnew-vdl)"
ssh "cp /tmp/server-linux-amd64 /home/<user>/ccnew-vdl-dev/ccnew-vdl && chmod +x ..."
sleep 8 && curl http://127.0.0.1:18000/api/config  # 验证版本
```
- 服务器 sudo 需要密码不可用，一律用 kill + systemd 自愈代替 systemctl。
- 注意：kill 与 cp 分两条 ssh 执行，合并执行常因连接中断导致 cp 没跑（校验 md5！）。

### adb / 真机调试
- Git Bash 下 adb shell 内的绝对路径必须加 `export MSYS_NO_PATHCONV=1`，
  否则 `/data/...` 被转成 `C:/Program Files/Git/data/...`。
- 小米翻盖机（Xiaomi 17 Pro Max）有内外双屏：screencap 要指定 `-d <displayId>`
  （`dumpsys SurfaceFlinger --display-id` 查询），否则可能截到黑屏副屏。
- 无线调试连接：`adb mdns services` 发现后自动连接，设备名形如 `adb-xxx._adb-tls-connect._tcp`。
