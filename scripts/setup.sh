#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 环境检查 & 自动安装 (Linux / macOS)
# Usage: ./scripts/setup.sh
# ────────────────────────────────────────────────────────
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}  ✓${NC} $*"; }
warn()  { echo -e "${YELLOW}  ⚠${NC} $*"; }
fail()  { echo -e "${RED}  ✗${NC} $*"; }

echo ""
echo "═══════════════════════════════════════"
echo "  ClashGo 环境检查"
echo "═══════════════════════════════════════"
echo ""

ERRORS=0

# Go
if command -v go >/dev/null 2>&1; then
    ok "Go: $(go version | awk '{print $3}')"
else
    fail "Go 未安装 → https://go.dev/dl/"
    ERRORS=$((ERRORS+1))
fi

# Node.js
if command -v node >/dev/null 2>&1; then
    ok "Node.js: $(node -v)"
else
    fail "Node.js 未安装 → https://nodejs.org/"
    ERRORS=$((ERRORS+1))
fi

# pnpm
if command -v pnpm >/dev/null 2>&1; then
    ok "pnpm: $(pnpm -v)"
else
    warn "pnpm 未安装，正在安装..."
    npm install -g pnpm && ok "pnpm: $(pnpm -v)" || { fail "pnpm 安装失败"; ERRORS=$((ERRORS+1)); }
fi

# Wails CLI
if command -v wails >/dev/null 2>&1; then
    ok "Wails CLI: $(wails version 2>/dev/null | head -1 || echo 'installed')"
else
    warn "Wails CLI 未安装，正在安装..."
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2 && ok "Wails CLI 安装完成" || { fail "Wails 安装失败"; ERRORS=$((ERRORS+1)); }
fi

# 平台特有检查
OS="$(uname -s)"
case "$OS" in
    Linux)
        info "检查 Linux 开发库..."
        if pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
            ok "webkit2gtk-4.1"
        else
            fail "webkit2gtk-4.1 未安装"
            echo "    Ubuntu/Debian: sudo apt install libwebkit2gtk-4.1-dev"
            echo "    Fedora:        sudo dnf install webkit2gtk4.1-devel"
            echo "    Arch:          sudo pacman -S webkit2gtk-4.1"
            ERRORS=$((ERRORS+1))
        fi
        if pkg-config --exists gtk+-3.0 2>/dev/null; then
            ok "GTK 3"
        else
            fail "GTK 3 开发库未安装"
            ERRORS=$((ERRORS+1))
        fi
        ;;
    Darwin)
        if xcode-select -p >/dev/null 2>&1; then
            ok "Xcode CLT: $(xcode-select -p)"
        else
            warn "Xcode Command Line Tools 未安装"
            echo "    运行: xcode-select --install"
            ERRORS=$((ERRORS+1))
        fi
        ;;
esac

echo ""
if [ "$ERRORS" -eq 0 ]; then
    echo -e "${GREEN}═══════════════════════════════════════${NC}"
    echo -e "${GREEN}  所有依赖已就绪! 🎉${NC}"
    echo -e "${GREEN}  运行 ./scripts/dev.sh 开始开发${NC}"
    echo -e "${GREEN}  运行 ./scripts/build.sh 进行构建${NC}"
    echo -e "${GREEN}═══════════════════════════════════════${NC}"
else
    echo -e "${RED}═══════════════════════════════════════${NC}"
    echo -e "${RED}  发现 $ERRORS 个问题，请先修复${NC}"
    echo -e "${RED}═══════════════════════════════════════${NC}"
    exit 1
fi
