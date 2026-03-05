# ClashGo Makefile
# 使用方法:
#   make dev             - 启动开发模式 (Wails hot-reload)
#   make build           - 构建当前平台生产版本
#   make build-all       - 构建全部平台 (Windows/Linux/macOS)
#   make build-windows   - 构建 Windows amd64
#   make build-linux     - 构建 Linux amd64
#   make build-linux-arm - 构建 Linux arm64 (树莓派/服务器)
#   make build-darwin    - 构建 macOS amd64
#   make build-darwin-arm- 构建 macOS arm64 (Apple Silicon M1/M2)
#   make package-deb     - 打包 .deb
#   make package-rpm     - 打包 .rpm
#   make clean           - 清理构建产物
#   make test            - 运行所有单元测试
#   make deps            - 下载/更新依赖

BINARY_NAME = clashgo
VERSION     = $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0-dev")
BUILD_TIME  = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || echo "unknown")
LDFLAGS     = -X main.Version=$(VERSION) \
              -X main.BuildTime=$(BUILD_TIME) \
              -s -w

WAILS       = wails
GO          = go
NFPM        = nfpm
DIST        = dist

.PHONY: all dev build build-all \
        build-windows build-linux build-linux-arm \
        build-darwin build-darwin-arm \
        deps install-tools clean test lint fmt \
        package-deb package-rpm version

## ─── 开发模式 ────────────────────────────────────────────────────────────────

dev:
	$(WAILS) dev

## ─── 当前平台构建 ─────────────────────────────────────────────────────────────

build:
	$(WAILS) build -ldflags "$(LDFLAGS)"
	@echo "✓ 构建完成 → build/bin/"

## ─── 多平台构建 ──────────────────────────────────────────────────────────────

# Windows 10/11 (amd64)
build-windows:
	$(WAILS) build -platform windows/amd64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-windows-amd64.exe
	@echo "✓ Windows amd64 → build/bin/"

# Linux (amd64) — 服务器/桌面
build-linux:
	$(WAILS) build -platform linux/amd64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64
	@echo "✓ Linux amd64 → build/bin/"

# Linux (arm64) — 树莓派 4 / AWS Graviton / Oracle Ampere
build-linux-arm:
	$(WAILS) build -platform linux/arm64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-arm64
	@echo "✓ Linux arm64 → build/bin/"

# macOS (amd64) — Intel Mac
build-darwin:
	$(WAILS) build -platform darwin/amd64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-darwin-amd64
	@echo "✓ macOS amd64 → build/bin/"

# macOS (arm64) — Apple Silicon M1/M2/M3
build-darwin-arm:
	$(WAILS) build -platform darwin/arm64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-darwin-arm64
	@echo "✓ macOS arm64 → build/bin/"

# macOS Universal Binary (amd64 + arm64)
build-darwin-universal:
	$(WAILS) build -platform darwin/universal -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-darwin-universal
	@echo "✓ macOS Universal → build/bin/"

## 一次构建全部平台
build-all: build-windows build-linux build-linux-arm build-darwin build-darwin-arm
	@echo ""
	@echo "✓ 所有平台构建完成 → build/bin/"
	@ls -lh build/bin/ 2>/dev/null || dir build\bin 2>nul || true

## ─── 仅 Go 编译检查（不含前端，快速验证代码）─────────────────────────────────

build-go:
	$(GO) build -ldflags "$(LDFLAGS)" ./...

build-go-windows:
	GOOS=windows GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" ./...

build-go-linux:
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" ./...

build-go-linux-arm:
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" ./...

build-go-darwin:
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" ./...

build-go-darwin-arm:
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" ./...

## ─── 依赖管理 ────────────────────────────────────────────────────────────────

# 下载所有依赖（首次初始化或 CI 用）
deps:
	$(GO) mod download
	$(GO) mod tidy
	@echo "✓ 依赖就绪"

# 更新依赖到最新版本
update-deps:
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "✓ 依赖已更新"

## ─── 工具安装 ────────────────────────────────────────────────────────────────

install-tools:
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
	$(GO) install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✓ 开发工具已安装: wails / nfpm / golangci-lint"

## ─── 测试 ────────────────────────────────────────────────────────────────────

test:
	$(GO) test -v -race -count=1 ./...

test-short:
	$(GO) test -short -count=1 ./...

## ─── 代码质量 ────────────────────────────────────────────────────────────────

fmt:
	$(GO) fmt ./...
	$(GO) vet ./...

lint:
	golangci-lint run ./...

## ─── 打包 ────────────────────────────────────────────────────────────────────

package-deb: build-linux
	$(NFPM) package --packager deb --config nfpm.yaml
	@echo "✓ .deb → dist/"

package-rpm: build-linux
	$(NFPM) package --packager rpm --config nfpm.yaml
	@echo "✓ .rpm → dist/"

## ─── 清理 ────────────────────────────────────────────────────────────────────

clean:
	rm -rf build/bin/ dist/
	$(GO) clean -cache
	@echo "✓ 清理完成"

## ─── 版本信息 ─────────────────────────────────────────────────────────────────

version:
	@echo "ClashGo  版本: $(VERSION)"
	@echo "构建时间: $(BUILD_TIME)"
	@$(GO) version
	@$(WAILS) version 2>/dev/null || echo "(wails 未安装)"
