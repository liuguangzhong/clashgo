#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 一键编译脚本 (macOS / Linux)
# Usage: bash scripts/build.sh [platform]
#   platform: linux/amd64 | darwin/amd64 | darwin/arm64 | windows/amd64
#   默认构建当前平台
# ────────────────────────────────────────────────────────
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/common.sh"

PLATFORM="${1:-}"

# ── 1. 环境检查 ───────────────────────────────────────
info "检查编译环境..."
check_env

# ── 2. 安装前端依赖 ───────────────────────────────────
install_frontend

# ── 3. Go 依赖 ────────────────────────────────────────
info "下载 Go 依赖..."
go mod download
ok "Go 依赖下载完成"

# ── 4. 构建 ──────────────────────────────────────────
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo 'v1.0.0-dev')"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -s -w"

detect_webkit_tags

if [ -n "$PLATFORM" ]; then
    info "构建目标平台: $PLATFORM"
    wails build -tags with_gvisor -platform "$PLATFORM" -ldflags "$LDFLAGS" $WAILS_TAGS
else
    info "构建当前平台..."
    wails build -tags with_gvisor -ldflags "$LDFLAGS" $WAILS_TAGS
fi

echo ""
ok "═══════════════════════════════════════"
ok "  ClashGo 构建成功! ($VERSION)"
ok "  产物目录: build/bin/"
ok "═══════════════════════════════════════"
ls -lh build/bin/ 2>/dev/null || true

# ── 5. 安装到系统路径 ────────────────────────────────
BINARY="$(find build/bin -maxdepth 1 -type f -executable 2>/dev/null | head -1)"
if [ -z "$BINARY" ]; then
    BINARY="build/bin/clashgo"
fi

if [ -f "$BINARY" ]; then
    echo ""
    info "安装到 /usr/local/bin/clashgo ..."
    sudo cp "$BINARY" /usr/local/bin/clashgo
    sudo chmod 755 /usr/local/bin/clashgo

    # 设置 TUN 模式需要的网络权限（免 sudo 运行）
    if command -v setcap >/dev/null 2>&1; then
        sudo setcap cap_net_admin,cap_net_raw,cap_net_bind_service=+ep /usr/local/bin/clashgo
        ok "已设置 TUN 网络权限 (cap_net_admin)"
    fi

    ok "已安装: /usr/local/bin/clashgo"
    echo ""
    ok "现在可以在任意位置运行: clashgo"
fi
