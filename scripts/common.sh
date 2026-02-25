#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 公共函数库 — build.sh / dev.sh 共用
# Usage: source scripts/common.sh
# ────────────────────────────────────────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# ── 环境检查 ──────────────────────────────────────────
check_env() {
    command -v go    >/dev/null 2>&1 || fail "Go 未安装。请先运行: bash scripts/setup.sh"
    command -v node  >/dev/null 2>&1 || fail "Node.js 未安装。请先运行: bash scripts/setup.sh"
    command -v pnpm  >/dev/null 2>&1 || fail "pnpm 未安装。请先运行: bash scripts/setup.sh"
    command -v wails >/dev/null 2>&1 || fail "Wails CLI 未安装。请先运行: bash scripts/setup.sh"

    # Linux 上检查开发库
    if [ "$(uname -s)" = "Linux" ] && command -v pkg-config >/dev/null 2>&1; then
        if ! pkg-config --exists webkit2gtk-4.1 2>/dev/null && ! pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
            fail "webkit2gtk 开发库未安装。请先运行: bash scripts/setup.sh"
        fi
        if ! pkg-config --exists gtk+-3.0 2>/dev/null; then
            fail "GTK 3 开发库未安装。请先运行: bash scripts/setup.sh"
        fi
    fi

    ok "Go $(go version | awk '{print $3}')"
    ok "Node $(node -v)"
    ok "pnpm $(pnpm -v)"
    ok "Wails $(wails version 2>/dev/null | head -1 || echo 'installed')"
}

# ── webkit2gtk 版本检测 ───────────────────────────────
detect_webkit_tags() {
    WAILS_TAGS=""
    if [ "$(uname -s)" = "Linux" ]; then
        if pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
            info "检测到 webkit2gtk-4.1，使用 -tags webkit2_41"
            WAILS_TAGS="-tags webkit2_41"
        elif pkg-config --exists webkit2gtk-4.0 2>/dev/null; then
            info "检测到 webkit2gtk-4.0，使用默认构建"
        else
            fail "未找到 webkit2gtk 开发库。请先运行: bash scripts/setup.sh"
        fi
    fi
}

# ── 前端依赖安装 ──────────────────────────────────────
install_frontend() {
    info "安装前端依赖..."
    cd frontend
    pnpm install --frozen-lockfile 2>/dev/null || pnpm install
    cd "$PROJECT_DIR"
    ok "前端依赖安装完成"
}
