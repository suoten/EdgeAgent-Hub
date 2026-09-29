# ============================================================
# Stage 1: 构建前端 (Vue3 → web/dist)
# ============================================================
FROM node:20-alpine AS frontend-builder

WORKDIR /build/web-vue

# 复制 package.json 先安装依赖 (利用 Docker 缓存)
COPY web-vue/package.json web-vue/package-lock.json ./
RUN npm ci --legacy-peer-deps || npm install --legacy-peer-deps

# 复制前端源码并构建
COPY web-vue/ ./
RUN npx vite build

# ============================================================
# Stage 2: 构建 Go 后端 (含前端静态文件)
# ============================================================
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# 复制 go.mod/go.sum 先下载依赖 (利用 Docker 缓存)
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 将前端构建产物复制到 web/dist (Go 代码引用此路径)
# vite.config.js 中 outDir: '../web/dist'，从 web-vue 构建后输出到 /build/web/dist
COPY --from=frontend-builder /build/web/dist ./web/dist

# 编译 (CGO_ENABLED=0 纯 Go，无 C 依赖)
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=docker" -o edgehub ./cmd/edgehub

# ── 运行时镜像 ──
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 复制二进制
COPY --from=builder /build/edgehub /app/edgehub

# 复制前端静态文件 (Go 运行时从文件系统读取)
COPY --from=builder /build/web/dist /app/web/dist

# 复制默认配置和示例工作流
COPY --from=builder /build/configs /app/configs
COPY --from=builder /build/workflows /app/workflows

# 复制 AI Sidecar
COPY --from=builder /build/ai_sidecar /app/ai_sidecar

ENV HUB_CONFIG=/app/configs/config.yaml
ENV TZ=Asia/Shanghai

EXPOSE 8080

ENTRYPOINT ["/app/edgehub"]
CMD ["-config", "/app/configs/config.yaml"]
