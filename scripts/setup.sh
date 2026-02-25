#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 环境检查 & 自动安装 (Linux / macOS)
# Usage: bash scripts/setup.sh
# ────────────────────────────────────────────────────────
# 不使用 set -e，让脚本能收集所有错误而不是遇到第一个就退出
set -uo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}  ✓${NC} $*"; }
warn()  { echo -e "${YELLOW}  ⚠${NC} $*"; }
fail()  { echo -e "${RED}  ✗${NC} $*"; }

echo ""
echo "═══════════════════════════════════════"
echo "  ClashGo 环境检查 & 依赖安装"
echo "═══════════════════════════════════════"
echo ""

ERRORS=0
HAS_GO=false
HAS_NODE=false

# ── 1. Go ──────────────────────────────────────────────
if command -v go >/dev/null 2>&1; then
    ok "Go: $(go version | awk '{print $3}')"
    HAS_GO=true
else
    fail "Go 未安装 → https://go.dev/dl/"
    ERRORS=$((ERRORS+1))
fi

# ── 2. Node.js ─────────────────────────────────────────
if command -v node >/dev/null 2>&1; then
    ok "Node.js: $(node -v)"
    HAS_NODE=true
else
    fail "Node.js 未安装 → https://nodejs.org/"
    ERRORS=$((ERRORS+1))
fi

# ── 3. pnpm (自动安装，依赖 npm/Node.js) ──────────────
if command -v pnpm >/dev/null 2>&1; then
    ok "pnpm: $(pnpm -v)"
else
    if [ "$HAS_NODE" = true ] && command -v npm >/dev/null 2>&1; then
        warn "pnpm 未安装，正在通过 npm 安装..."
        if npm install -g pnpm >/dev/null 2>&1 && command -v pnpm >/dev/null 2>&1; then
            ok "pnpm: $(pnpm -v)"
        else
            fail "pnpm 自动安装失败，请手动运行: npm install -g pnpm"
            ERRORS=$((ERRORS+1))
        fi
    else
        fail "pnpm 未安装 (需要先安装 Node.js 后才能自动安装)"
        ERRORS=$((ERRORS+1))
    fi
fi

# ── 4. Wails CLI (自动安装，依赖 Go) ──────────────────
if command -v wails >/dev/null 2>&1; then
    ok "Wails CLI: $(wails version 2>/dev/null | head -1 || echo 'installed')"
else
    if [ "$HAS_GO" = true ]; then
        warn "Wails CLI 未安装，正在通过 go install 安装..."
        if go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2 2>&1; then
            export PATH="$(go env GOPATH)/bin:$PATH"
            if command -v wails >/dev/null 2>&1; then
                ok "Wails CLI 安装完成"
            else
                fail "Wails CLI 安装完成但未在 PATH 中找到，请将 $(go env GOPATH)/bin 加入 PATH"
                ERRORS=$((ERRORS+1))
            fi
        else
            fail "Wails CLI 安装失败"
            ERRORS=$((ERRORS+1))
        fi
    else
        fail "Wails CLI 未安装 (需要先安装 Go 后才能自动安装)"
        ERRORS=$((ERRORS+1))
    fi
fi

# ── 5. 平台特有检查 ───────────────────────────────────
OS="$(uname -s)"
case "$OS" in
    Linux)
        info "检查 Linux 开发库..."
        if command -v pkg-config >/dev/null 2>&1; then
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
                echo "    Ubuntu/Debian: sudo apt install libgtk-3-dev"
                echo "    Fedora:        sudo dnf install gtk3-devel"
                echo "    Arch:          sudo pacman -S gtk3"
                ERRORS=$((ERRORS+1))
            fi
        else
            fail "pkg-config 未安装"
            echo "    Ubuntu/Debian: sudo apt install pkg-config"
            echo "    Fedora:        sudo dnf install pkgconfig"
            echo "    Arch:          sudo pacman -S pkgconf"
            ERRORS=$((ERRORS+1))
        fi
        ;;
    Darwin)
        if xcode-select -p >/dev/null 2>&1; then
            ok "Xcode CLT: $(xcode-select -p)"
        else
            fail "Xcode Command Line Tools 未安装"
            echo "    运行: xcode-select --install"
            ERRORS=$((ERRORS+1))
        fi
        ;;
esac

# ── 结果汇总 ──────────────────────────────────────────
echo ""
if [ "$ERRORS" -eq 0 ]; then
    echo -e "${GREEN}═══════════════════════════════════════${NC}"
    echo -e "${GREEN}  所有依赖已就绪! 🎉${NC}"
    echo -e "${GREEN}  运行 bash scripts/dev.sh 开始开发${NC}"
    echo -e "${GREEN}  运行 bash scripts/build.sh 进行构建${NC}"
    echo -e "${GREEN}═══════════════════════════════════════${NC}"
else
    echo -e "${RED}═══════════════════════════════════════${NC}"
    echo -e "${RED}  发现 $ERRORS 个问题，请先修复后重新运行${NC}"
    echo -e "${RED}  bash scripts/setup.sh${NC}"
    echo -e "${RED}═══════════════════════════════════════${NC}"
    exit 1
fi
