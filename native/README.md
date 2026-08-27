# CCNEW 视频下载器 - Windows 原生版

纯 Rust + Slint 原生 GUI，无 WebView2 / PowerShell 依赖。

## 与 Go 版的隔离

| 项目 | Go 版 | Rust 原生版 |
|------|-------|-------------|
| 进程名 | server.exe | ccnew-vdl-native.exe |
| 端口 | 18000 | 18001 |
| 安装目录 | 抖音B站视频解析工具 | CCNEW-VideoDownloader-Native |
| 数据目录 | %APPDATA%\ccnew-vdl | %LOCALAPPDATA%\CCNEW-VideoDownloader |
| 可共存 | ✓ | ✓ |

## 构建

```bash
cd native
cargo build --release
```

## 开发状态

骨架阶段，UI 框架和项目结构已就位，核心功能待移植。