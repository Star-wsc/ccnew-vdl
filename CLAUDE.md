# DouBi (CCNEW-VideoDownloader) 开发规则

## 版本号规则（强制）

**每次迭代必须更新版本号，禁止"改了代码不改版本"。移动端与服务器端版本号独立迭代、互不关联。**

### 服务器版（Go）
- 版本号仅在构建时注入：`-X main.Version=X.Y.Z`
- 当前版本：**1.4.11**（独立迭代）
- 迭代一次 +0.0.1（功能更新 +0.1.0 自定），每次重新部署必须带新版本号
- 验证：部署后 `curl http://<server>:18000/api/config` 里 `version` 字段必须变化

### 移动版（Flutter/Android）
- 版本号唯一来源：`android_flutter/pubspec.yaml` 的 `version: X.Y.Z+N`
  - X.Y.Z 给用户看（设置页-关于、更新检查比对用）
  - N 是 versionCode，**每次发版必须递增**（Android 覆盖安装/更新要求）
- 当前版本：**1.5.0+2**（独立迭代，不跟服务器版对齐）
- 迭代时同步检查三处一致性：
  1. `pubspec.yaml`（唯一来源）
  2. 设置页-关于 显示的版本（运行时从 PackageManager 读取，禁止硬编码）
  3. GitHub Release 的 tag（格式 `app-vX.Y.Z`，供 APP 检查更新比对）

### 迭代清单（每次发版过一遍）
- [ ] 明确本次迭代是移动版还是服务器版（或两者）
- [ ] 对应端版本号 +1，versionCode 递增（移动版）
- [ ] 移动版：`flutter build apk` 后检查 APK 内版本
- [ ] 服务器版：重新编译部署后 curl 确认 /api/config 版本变化
- [ ] 更新检查比对逻辑依赖的 tag 格式不破坏

## 命名规则
- 项目对外名称统一 **DouBi**（APP名、Web控制台标题、相册目录 DouBi-Downloader）
- 仓库名保留 CCNEW-VideoDownloader（历史原因），用户可见处禁止再出现 CCNEW

## 架构备忘
- 服务器：Go + Gin，Ubuntu <server-ip>:18000，systemd 服务名 ccnew-vdl，二进制 /home/<user>/ccnew-vdl-dev/ccnew-vdl
- 移动端：Flutter 瘦客户端，只做 UI/相册/播放，全部解析下载在服务器
- APP任务隔离：所有 APP 创建的任务带 `source=app`，Web端不受影响
- 数据策略：本地缓存优先（LocalStore），事件驱动刷新（切菜单/操作触发，1分钟CD）+ 5分钟兜底 + 下载中2秒实时；后台零轮询
- 部署：交叉编译 `GOOS=linux GOARCH=amd64`，scp 到 /tmp 后 kill+cp+systemd 自动拉起
- Flutter 构建：先把 `~/.gradle/init.gradle` 改名避开阿里云镜像冲突，构建完恢复

## 更新检查
- APP 通过 GitHub Releases API 检查移动版更新，只认 tag 前缀 `app-v` 的 release
- 服务器版不检查更新（手动部署）
