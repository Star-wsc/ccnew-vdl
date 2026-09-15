FROM golang:1.22-alpine AS builder

WORKDIR /app

# 安装git和ca-certificates
RUN apk add --no-cache git ca-certificates

# 复制go.mod和go.sum
COPY go.mod go.sum* ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译
ARG VERSION=v1.3.5
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.Version=${VERSION}" -o server ./cmd/server/

# 运行阶段（必须用glibc基础镜像，yt-dlp二进制是glibc编译的）
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制编译好的二进制文件
COPY --from=builder /app/server .

# 复制静态文件
COPY --from=builder /app/static ./static

# YouTube engine: yt-dlp (select by TARGETARCH)
ARG TARGETARCH
COPY --from=builder /app/yt-dlp/yt-dlp-linux-amd64 /app/yt-dlp/yt-dlp-linux-amd64
COPY --from=builder /app/yt-dlp/yt-dlp-linux-arm64 /app/yt-dlp/yt-dlp-linux-arm64
RUN mkdir -p /app/yt-dlp && chmod +x /app/yt-dlp/yt-dlp-linux-* && \
    if [ "$TARGETARCH" = "arm64" ]; then cp /app/yt-dlp/yt-dlp-linux-arm64 /app/yt-dlp/yt-dlp; else cp /app/yt-dlp/yt-dlp-linux-amd64 /app/yt-dlp/yt-dlp; fi

# 创建下载目录
RUN mkdir -p /downloads /logs

# 设置环境变量
ENV PORT=18000
ENV DOWNLOAD_DIR=/downloads
ENV LOG_DIR=/logs

# 暴露端口
EXPOSE 18000

# 运行
CMD ["./server"]
