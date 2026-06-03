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
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/

# 运行阶段
FROM alpine:latest

RUN apk add --no-cache ca-certificates ffmpeg

WORKDIR /app

# 复制编译好的二进制文件
COPY --from=builder /app/server .

# 复制静态文件
COPY --from=builder /app/static ./static

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
