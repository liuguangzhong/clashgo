#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# ClashGo 一键编译脚本 (macOS / Linux)
# 用法:
#   bash scripts/build.sh                      # 编译当前平台
#   bash scripts/build.sh --all                # 编译全部平台
#   bash scripts/build.sh --platform linux/arm64
#   bash scripts/build.sh --go-only            # 仅 go build，快速验证
#   bash scripts/build.sh --deps               # 仅更新依赖
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# ── 颜色 ──────────────────────────────────────────────────────────────────────
info()  { echo -e "\033[36m  $*\033[0m"; }
ok()    { echo -e "\033[32m✓ $*\033[0m"; }
warn()  { echo -e "\033[33m! $*\033[0m"; }
fail()  { echo -e "\033[31m✗ $*\033[0m"; exit 1; }
title() { echo -e "\n\033[35m══ $* \033[0m"; }

# ── 参数解析 ──────────────────────────────────────────────────────────────────
PLATFORM=""
ALL=0
GO_ONLY=0
DEPS_ONLY=0

for arg in "$@"; do
    case "$arg" in
        --all)           ALL=1 ;;
        --go-only)       GO_ONLY=1 ;;
        --deps)          DEPS_ONLY=1 ;;
        --platform=*)    PLATFORM="${arg#*=}" ;;
        -p)              shift; PLATFORM="$1" ;;
        --help|-h)
            cat <<'EOF'
ClashGo 一键编译脚本
------------------------------------------------------------
用法: bash scripts/build.sh [选项]

选项:
  --platform=<平台>  目标平台 (linux/amd64 | linux/arm64 |
                              darwin/amd64 | darwin/arm64 | windows/amd64)
  --all              编译全部平台
  --go-only          仅 go build（不含前端，快速验证）
  --deps             只下载/更新依赖
  --help             显示此帮助

示例:
  bash scripts/build.sh
  bash scripts/build.sh --platform linux/arm64
  bash scripts/build.sh --all
  bash scripts/build.sh --go-only
EOF
            exit 0
            ;;
    esac
done

# ── 版本信息 ──────────────────────────────────────────────────────────────────
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo 'v1.0.0-dev')"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || echo 'unknown')"
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -s -w"

title "ClashGo ${VERSION} (${BUILD_TIME})"

# ── 仅更新依赖 ────────────────────────────────────────────────────────────────
if [ "$DEPS_ONLY" = "1" ]; then
    title "更新依赖"
    info "go mod download..."
    go mod download
    info "go mod tidy..."
    go mod tidy
    ok "依赖就绪"
    exit 0
fi

# ── 环境检查 ──────────────────────────────────────────────────────────────────
title "环境检查"

command -v go   >/dev/null 2>&1 || fail "Go 未安装，请访问 https://go.dev/dl/"
ok "Go: $(go version)"

if [ "$GO_ONLY" = "0" ]; then
    if ! command -v wails >/dev/null 2>&1; then
        warn "Wails 未安装，正在安装..."
        go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
        ok "Wails 已安装"
    else
        ok "Wails: $(wails version 2>/dev/null | head -1)"
    fi

    command -v node >/dev/null 2>&1 || fail "Node.js 未安装，请访问 https://nodejs.org"
    ok "Node.js: $(node --version)"

    # 前端依赖
    if [ -f "frontend/package.json" ]; then
        PM="npm"
        command -v pnpm >/dev/null 2>&1 && PM="pnpm"
        info "安装前端依赖 (${PM} install)..."
        $PM install --prefix frontend
        ok "前端依赖安装完成"
    fi
fi

# Go 依赖
info "go mod download..."
go mod download
go mod tidy
ok "Go 依赖就绪"

# ── 构建函数 ──────────────────────────────────────────────────────────────────
mkdir -p build/bin

build_platform() {
    local plat="$1"
    local goos="${plat%/*}"
    local goarch="${plat#*/}"
    local out="clashgo-${goos}-${goarch}"
    [ "$goos" = "windows" ] && out="${out}.exe"

    title "构建 ${plat} → ${out}"

    if [ "$GO_ONLY" = "1" ]; then
        GOOS="$goos" GOARCH="$goarch" go build -ldflags "$LDFLAGS" -o "build/bin/${out}" .
    else
        wails build -platform "$plat" -ldflags "$LDFLAGS" -o "$out"
    fi

    ok "${plat} 构建完成 → build/bin/${out}"
}

# ── 执行构建 ──────────────────────────────────────────────────────────────────
if [ "$ALL" = "1" ]; then
    for plat in windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
        build_platform "$plat"
    done

elif [ -n "$PLATFORM" ]; then
    build_platform "$PLATFORM"

else
    # 默认：当前平台
    if [ "$GO_ONLY" = "1" ]; then
        title "Go only 编译"
        go build -ldflags "$LDFLAGS" ./...
    else
        # 自动检测当前系统
        CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        CURRENT_ARCH=$(uname -m)
        [ "$CURRENT_ARCH" = "x86_64" ] && CURRENT_ARCH="amd64"
        [ "$CURRENT_ARCH" = "aarch64" ] && CURRENT_ARCH="arm64"
        build_platform "${CURRENT_OS}/${CURRENT_ARCH}"
    fi
fi

# ── 展示产物 ──────────────────────────────────────────────────────────────────
title "构建产物"
if [ -d "build/bin" ]; then
    ls -lh build/bin/ 2>/dev/null | tail -n +2 | while read -r line; do
        ok "$line"
    done

    # Linux TUN 权限
    for bin in build/bin/clashgo-linux-*; do
        if [ -f "$bin" ] && command -v setcap >/dev/null 2>&1; then
            sudo setcap cap_net_admin,cap_net_raw,cap_net_bind_service=+ep "$bin" 2>/dev/null || true
            ok "已设置 TUN 权限: $(basename $bin)"
        fi
    done
fi

echo ""
ok "══════════════════════════════════════════"
ok "  ClashGo 构建完成！版本: ${VERSION}"
ok "  输出目录: build/bin/"
ok "══════════════════════════════════════════"
