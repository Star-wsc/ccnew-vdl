# CCNEW-VideoDownloader

一个支持B站4K视频下载和抖音视频下载的工具。

## 功能特性

- **B站4K视频下载**：支持DASH格式音视频分离流，M4S头部剥离 + FFmpeg合并
- **抖音视频下载**：支持单视频和合集解析
- **多部署形态**：Docker版、CLI服务器版、桌面版

## 快速开始

### CLI服务器版

```bash
# 编译
go build -o server ./cmd/server/

# 运行
./server
```

访问 http://127.0.0.1:18000

### Docker版

```bash
# 构建镜像
docker build -t video-downloader .

# 运行容器
docker run -d -p 18000:18000 -v ./downloads:/downloads video-downloader
```

或者使用docker-compose：

```bash
docker-compose up -d
```

### 桌面版

```bash
# 编译（与服务器版同一入口）
go build -ldflags "-H=windowsgui" -o server.exe ./cmd/server/

# 运行（自动拉起 WebView2 桌面窗口，需 webview2/ 目录）
./start-desktop.bat
```

## 配置

### 环境变量

- `PORT`：服务器端口（默认：18000）
- `DOWNLOAD_DIR`：下载目录（默认：~/Downloads/video-downloader）
- `LOG_DIR`：日志目录（默认：./logs）
- `BILIBILI_COOKIE`：B站Cookie（可选，用于4K下载）
- `DOUYIN_COOKIE`：抖音Cookie（可选）
- `PROXY`：代理设置（可选）

### 配置文件

配置文件位于 `~/.config/ccnew-vdl/config.json`

```json
{
  "port": "18000",
  "download_dir": "/path/to/downloads",
  "bilibili_cookie": "your_cookie_here",
  "douyin_cookie": "your_cookie_here"
}
```

## B站4K视频下载说明

B站4K视频使用DASH格式，音视频分离。下载流程：

1. 获取视频流和音频流URL
2. 分别下载视频流和音频流（.m4s格式）
3. 剥离M4S文件的8字节私有头部
4. 使用FFmpeg合并音视频流

**注意**：4K视频需要B站大会员Cookie才能下载。

## API接口

### 预览链接（不创建任务，返回单视频或合集信息）

```http
POST /api/tasks
Content-Type: application/json

{
  "url": "https://www.bilibili.com/video/BV1xx411c7mD",
  "quality": "preview"
}
```

### 创建下载任务

```http
POST /api/tasks
Content-Type: application/json

{
  "url": "https://www.bilibili.com/video/BV1xx411c7mD",
  "quality": "1080p"
}
```

### 获取任务列表

```http
GET /api/tasks
```

### 删除任务

```http
DELETE /api/tasks/:id?deleteFile=true
```

### 下载文件

```http
GET /api/tasks/:id/download
```

## 支持的画质

### B站
- 4K (需要大会员)
- 1080P
- 720P
- 480P

### 抖音
- 默认画质

## 开发

### 项目结构

```
├── cmd/
│   ├── server/          # CLI服务器版
│   └── desktop/         # 桌面版
├── internal/
│   ├── bilibili/        # B站下载相关
│   ├── douyin/          # 抖音下载相关
│   ├── download/        # 下载管理器
│   └── config/          # 配置管理
├── static/              # 前端界面
├── Dockerfile
└── docker-compose.yml
```

### 依赖

- Go 1.22+
- FFmpeg（用于音视频合并）
- Gin（HTTP框架）

## 许可证

MIT License
