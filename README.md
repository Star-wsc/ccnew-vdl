# DouBi 视频下载器

<p align="center">
  <a href="https://github.com/Star-wsc/ccnew-vdl/releases"><img alt="Release" src="https://img.shields.io/github/v/release/Star-wsc/ccnew-vdl?include_prereleases&label=%E6%9C%8D%E5%8A%A1%E7%AB%AF"></a>
  <a href="https://github.com/Star-wsc/ccnew-vdl/releases?q=app-v"><img alt="Android" src="https://img.shields.io/github/v/release/Star-wsc/ccnew-vdl?filter=app-v*&label=Android"></a>
  <a href="https://hub.docker.com/r/wsc768043912/ccnew-vdl"><img alt="Docker" src="https://img.shields.io/docker/pulls/wsc768043912/ccnew-vdl?label=Docker%20Pulls"></a>
</p>

**DouBi（抖B）** —— 抖音 + B站视频下载器。支持单视频与合集批量下载、DASH 音视频自动合并、订阅自动追更。一套 Go 服务端跑在 NAS / 电脑上，全家设备共用：浏览器 Web 控制台、Windows 桌面版、Android APP。

> ⚠️ 仅供个人学习研究，请勿用于商业用途，下载内容版权归原作者所有。

---

## ✨ 功能

**服务端（所有客户端共用）**
- 🎬 抖音单视频 / 合集解析下载，多策略解析 + 自动重试，无需登录即可下载
- 📺 B站视频下载，支持 4K/2K/1080p（清晰度取决于账号权限，DASH 自动合并音视频）
- 📚 合集批量下载 + **订阅模式**：自动检查 UP 主更新并追新
- 🔗 短链支持：`v.douyin.com`、`b23.tv` 分享链接直接粘贴
- 🌐 Web 控制台：粘贴链接、进度监控、合集管理、**在线播放**（支持拖动进度）

**Android APP（DouBi）**
- 📱 原生液态玻璃 UI，深色/浅色模式
- 💾 一键保存到系统相册（自动写入媒体库）
- ▶️ 内置播放器（本地缓存 / 在线流式播放，可拖动进度）
- 🔄 与 Web 任务互不干扰，后端共用
- 📴 本地缓存优先 + 事件驱动刷新：省流量、省电，后台零请求
- ⬆️ 应用内检查更新，直接下载安装

---

## 🚀 快速开始

### 方式一：Docker 部署（NAS / Linux 推荐）

```bash
mkdir douby && cd douby
curl -O https://raw.githubusercontent.com/Star-wsc/ccnew-vdl/main/docker-compose.yml
docker compose up -d
```

浏览器访问 `http://NAS的IP:18000` 即可使用。

<details>
<summary>不用 compose，单命令运行</summary>

```bash
docker run -d --name douby -p 18000:18000 \
  -v ./downloads:/downloads \
  -v ./config:/root/.config/ccnew-vdl \
  -v ./logs:/logs \
  --restart unless-stopped \
  wsc768043912/ccnew-vdl:latest
```
</details>

### 方式二：Windows 桌面版

到 [Releases](https://github.com/Star-wsc/ccnew-vdl/releases) 下载 `ccnew-vdl-setup.exe` 安装，启动后自动打开桌面窗口（基于 WebView2）。

### 方式三：Android APP

到 [Releases](https://github.com/Star-wsc/ccnew-vdl/releases?q=app-v) 下载 `DouBi-vX.Y.Z.apk` 安装，首次打开填入服务器地址（如 `http://192.168.x.x:18000`）即可。

- APP 与服务器同一局域网内使用；公网使用请自行做好反向代理与访问控制
- APP 内「设置 → 检查更新」可检测新版本并直接下载安装

---

## ⚙️ Cookie 配置（可选但建议）

| 平台 | 不配置 | 配置后 |
|---|---|---|
| B站 | 最高 480P/720P | 账号可用最高画质（大会员 4K） |
| 抖音 | 大部分视频可下 | 解析更稳、部分视频解锁 |

配置方式任选：
1. Web 控制台 → 设置 → 粘贴 Cookie
2. docker-compose.yml 里填 `BILIBILI_COOKIE` / `DOUYIN_COOKIE` 环境变量

获取方法：浏览器登录 B站/抖音 → F12 开发者工具 → Network → 复制请求头中完整的 `Cookie` 值。

---

## 🏗️ 架构

```
浏览器 Web 控制台 ─┐
Windows 桌面版    ─┼──→ Go 服务端 (Docker/裸跑)
Android APP      ─┘         │
                            ├─ 抖音/B站解析（多策略+重试）
                            ├─ DASH 下载 + FFmpeg 合并
                            └─ 合集管理 / 订阅刷新 / 操作日志
```

- 服务端版本（`v*`）与移动端版本（`app-v*`）**独立迭代**
- 环境变量：`PORT`（默认18000）、`DOWNLOAD_DIR`、`LOG_DIR`、`BILIBILI_COOKIE`、`DOUYIN_COOKIE`
- 配置文件（Cookie 等）持久化在 `/root/.config/ccnew-vdl/config.json`

---

## ❓ 常见问题

<details>
<summary>下载的视频没有声音？</summary>

升级到 v1.4.6+。旧版本对 DASH 分离流的音频合并有缺陷，当前版本会自动下载音频流并用 FFmpeg 合并。
</details>

<details>
<summary>抖音链接解析失败？</summary>

分享文本里混杂的文字/表情不影响，直接整段粘贴即可。若持续失败：配置抖音 Cookie 后重试；服务端对解析失败有 5 次自动重试。
</details>

<details>
<summary>B站 4K 下载不了？</summary>

4K 需要登录 B站账号且具有大会员资格，Cookie 未配置或账号无权限时自动降级到可用最高画质。
</details>

<details>
<summary>APP 连不上服务器？</summary>

确认手机与服务器在同一局域网、地址填 `http://IP:端口`（带 http:// 前缀）。APP 支持离线查看上次缓存的任务列表。
</details>

---

## 🛠️ 开发

```bash
# 服务端
go build -o server ./cmd/server/

# Android APP（Flutter 3.41+）
cd android_flutter && flutter build apk
```

技术栈：Go 1.22 + Gin + FFmpeg ｜ Flutter + Dart ｜ WebView2（Windows 桌面壳）

详细开发文档见 [DEVELOPMENT.md](DEVELOPMENT.md)，项目规则见 [CLAUDE.md](CLAUDE.md)。

---

## 📄 License

仅供学习交流使用。视频内容版权归原平台及作者所有，请于下载后 24 小时内自行删除。
