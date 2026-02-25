#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 一键编译脚本 (macOS / Linux)
# Usage: bash scripts/build.sh [platform]
#   platform: linux/amd64 | darwin/amd64 | darwin/arm64 | windows/amd64
#   默认构建当前平台
# ────────────────────────────────────────────────────────
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PLATFORM="${1:-}"

cd "$PROJECT_DIR"

# ── 1. 环境检查 — 缺失则提示跑 setup ─────────────────
info "检查编译环境..."

command -v go    >/dev/null 2>&1 || fail "Go 未安装。请先运行: bash scripts/setup.sh"
command -v node  >/dev/null 2>&1 || fail "Node.js 未安装。请先运行: bash scripts/setup.sh"
command -v pnpm  >/dev/null 2>&1 || fail "pnpm 未安装。请先运行: bash scripts/setup.sh"
command -v wails >/dev/null 2>&1 || fail "Wails CLI 未安装。请先运行: bash scripts/setup.sh"

ok "Go $(go version | awk '{print $3}')"
ok "Node $(node -v)"
ok "pnpm $(pnpm -v)"
ok "Wails $(wails version 2>/dev/null | head -1 || echo 'installed')"

# ── 2. 安装前端依赖 ───────────────────────────────────
info "安装前端依赖..."
cd frontend
pnpm install --frozen-lockfile 2>/dev/null || pnpm install
cd "$PROJECT_DIR"
ok "前端依赖安装完成"

# ── 3. Go 依赖 ─────────────────────────────────────────
info "下载 Go 依赖..."
go mod download
ok "Go 依赖下载完成"

# ── 4. 构建 ──────────────────────────────────────────
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo 'v1.0.0-dev')"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -s -w"

if [ -n "$PLATFORM" ]; then
    info "构建目标平台: $PLATFORM"
    wails build -platform "$PLATFORM" -ldflags "$LDFLAGS"
else
    info "构建当前平台..."
    wails build -ldflags "$LDFLAGS"
fi

echo ""
ok "═══════════════════════════════════════"
ok "  ClashGo 构建成功! ($VERSION)"
ok "  产物目录: build/bin/"
ok "═══════════════════════════════════════"
ls -lh build/bin/ 2>/dev/null || true
