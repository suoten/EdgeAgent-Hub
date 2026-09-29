.PHONY: build run test vet clean docker cli selftest init tidy frontend all

VERSION ?= 1.0.0
LDFLAGS = -ldflags="-s -w -X main.version=$(VERSION)"

# ── 构建前端 (Vue3 → web/dist) ──
frontend:
	cd web-vue && npm install --legacy-peer-deps && npx vite build

# ── 构建 (需先构建前端) ──
build: frontend
	go build $(LDFLAGS) -o dist/edgehub ./cmd/edgehub

# ── 仅编译 Go 二进制 (跳过前端, 用于开发) ──
build-go:
	go build $(LDFLAGS) -o dist/edgehub ./cmd/edgehub

# ── 运行 ──
run: build
	./dist/edgehub -config configs/config.yaml

# ── 测试 ──
test:
	go test -v -race -cover ./...

# ── 代码检查 ──
vet:
	go vet ./...

# ── 清理 ──
clean:
	rm -rf dist/ coverage.out web/dist/

# ── 全量构建 (前端 + 后端 + 测试) ──
all: frontend build vet test

# ── Docker 构建 ──
docker:
	docker build -t edgelite/edgeagent-hub:$(VERSION) .
	docker build -t edgelite/edgeagent-hub-sidecar:$(VERSION) -f Dockerfile.sidecar .

# ── Docker Compose 启动 ──
up:
	docker compose up -d

# ── Docker Compose 停止 ──
down:
	docker compose down

# ── 生成依赖 ──
tidy:
	go mod tidy

# ── CLI 工具 ──
cli: build
	./dist/edgehub selftest

# ── 自检 ──
selftest: build
	./dist/edgehub selftest

# ── 初始化 ──
init: build
	./dist/edgehub init

# ── 发布打包 ──
release:
	powershell -ExecutionPolicy Bypass -File make-release.ps1 -Version $(VERSION)
