# DouBi 视频下载器（Docker 版）

抖音 + B站视频下载器。支持单视频与合集批量下载、DASH 音视频自动合并、订阅自动追更。
Web 控制台直接粘贴链接使用，配套 Windows 桌面版与 Android APP（[GitHub Releases](https://github.com/Star-wsc/ccnew-vdl/releases)）。

## 快速开始

```bash
mkdir douby && cd douby
curl -O https://raw.githubusercontent.com/Star-wsc/ccnew-vdl/main/docker-compose.yml
docker compose up -d
```

浏览器访问 `http://NAS的IP:18000` 即可使用。支持群晖 / 飞牛 / Unraid / 极空间 / 普通Linux。

单命令运行：

```bash
docker run -d --name douby -p 18000:18000 \
  -v ./downloads:/downloads \
  -v ./config:/root/.config/ccnew-vdl \
  -v ./logs:/logs \
  --restart unless-stopped \
  wsc768043912/ccnew-vdl:latest
```

## 卷挂载

| 容器路径 | 用途 |
|---|---|
| `/downloads` | 视频保存目录 |
| `/root/.config/ccnew-vdl` | Cookie 等配置持久化 |
| `/logs` | 日志与任务记录 |

## 环境变量

| 变量 | 说明 |
|---|---|
| `PORT` | 监听端口（默认 18000） |
| `DOWNLOAD_DIR` | 下载目录（容器内，默认 /downloads） |
| `LOG_DIR` | 日志目录（容器内，默认 /logs） |
| `BILIBILI_COOKIE` | B站 Cookie（可选，配置后解锁账号最高画质） |
| `DOUYIN_COOKIE` | 抖音 Cookie（可选，解析更稳） |

## 功能

- 抖音单视频/合集下载（无需登录）
- B站视频下载，DASH 自动合并，账号权限内最高画质（大会员 4K）
- 合集订阅：自动检查更新并下载新集
- Web 控制台在线播放
- `v.douyin.com` / `b23.tv` 短链直接粘贴

多架构镜像：linux/amd64 + linux/arm64（群晖/树莓派/NAS均可）。

⚠️ 仅供个人学习研究，下载内容版权归原作者所有。
