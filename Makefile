# ClashGo Makefile
# 使用方法:
#   make dev          - 启动开发模式 (Wails hot-reload)
#   make build        - 构建生产版本
#   make build-linux  - 交叉编译 Linux (在 Windows 上)
#   make package-deb  - 打包 .deb
#   make package-rpm  - 打包 .rpm  
#   make clean        - 清理构建产物
#   make test         - 运行所有单元测试

BINARY_NAME = clashgo
VERSION     = $(shell git describe --tags --always --dirty 2>/dev/null || echo "v1.0.0-dev")
BUILD_TIME  = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     = -X clashgo/internal/config.EmbeddedMihomoVersion=v1.18.10 \
              -X main.Version=$(VERSION) \
              -X main.BuildTime=$(BUILD_TIME) \
              -s -w

WAILS       = wails
GO          = go
NFPM        = nfpm

.PHONY: all dev build build-linux build-windows install-tools clean test lint fmt package-deb package-rpm

## 开发模式（带热更新）
dev:
	$(WAILS) dev

## 生产构建（当前平台）
build:
	$(WAILS) build -tags with_gvisor -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)

## 交叉编译 Linux amd64（可在 Windows 上运行）
build-linux:
	$(WAILS) build -tags with_gvisor -platform linux/amd64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64

## 交叉编译 Windows amd64
build-windows:
	$(WAILS) build -tags with_gvisor -platform windows/amd64 -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-windows-amd64.exe

## 仅编译 Go 代码（快速检查，不含前端）
build-go:
	$(GO) build -tags with_gvisor -ldflags "$(LDFLAGS)" ./...

## 运行所有单元测试
test:
	$(GO) test -v -race -count=1 ./...

## 运行单元测试（短超时）
test-short:
	$(GO) test -short -count=1 ./...

## 代码格式化
fmt:
	$(GO) fmt ./...
	$(GO) vet ./...

## 代码检查
lint:
	golangci-lint run ./...

## 打包 .deb
package-deb: build-linux
	$(NFPM) package --packager deb --config nfpm.yaml
	@echo "✓ .deb package created in dist/"

## 打包 .rpm
package-rpm: build-linux
	$(NFPM) package --packager rpm --config nfpm.yaml
	@echo "✓ .rpm package created in dist/"

## 安装开发工具
install-tools:
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
	$(GO) install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✓ Development tools installed"

## 更新所有依赖
update-deps:
	$(GO) get -u ./...
	$(GO) mod tidy

## 清理构建产物
clean:
	rm -rf build/bin/
	rm -rf dist/
	$(GO) clean -cache

## 显示版本信息
version:
	@echo "ClashGo $(VERSION)"
	@echo "Build: $(BUILD_TIME)"
	@$(GO) version
	@$(WAILS) version
