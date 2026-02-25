#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 一键启动脚本 (开发模式, macOS / Linux)
# Usage: bash scripts/dev.sh
# ────────────────────────────────────────────────────────
set -euo pipefail

CYAN='\033[0;36m'; GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
info() { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()   { echo -e "${GREEN}[OK]${NC}    $*"; }
fail() { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

# 环境检查 — 全部检查，缺失则提示跑 setup
command -v go    >/dev/null 2>&1 || fail "Go 未安装。请先运行: bash scripts/setup.sh"
command -v node  >/dev/null 2>&1 || fail "Node.js 未安装。请先运行: bash scripts/setup.sh"
command -v pnpm  >/dev/null 2>&1 || fail "pnpm 未安装。请先运行: bash scripts/setup.sh"
command -v wails >/dev/null 2>&1 || fail "Wails CLI 未安装。请先运行: bash scripts/setup.sh"

# 确保前端依赖
if [ ! -d "frontend/node_modules" ]; then
    info "安装前端依赖..."
    cd frontend && pnpm install && cd "$PROJECT_DIR"
fi

ok "环境就绪，启动开发模式..."
echo ""
echo "  📦 前端热更新: Vite HMR"
echo "  🔧 后端热更新: Wails Auto-rebuild"
echo "  🌐 按 Ctrl+C 停止"
echo ""

exec wails dev
