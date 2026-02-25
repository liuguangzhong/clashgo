#!/usr/bin/env bash
# ────────────────────────────────────────────────────────
# ClashGo 一键启动脚本 (开发模式, macOS / Linux)
# Usage: bash scripts/dev.sh
# ────────────────────────────────────────────────────────
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/common.sh"

# 环境检查
check_env

# 确保前端依赖
if [ ! -d "frontend/node_modules" ]; then
    install_frontend
fi

ok "环境就绪，启动开发模式..."
echo ""
echo "  📦 前端热更新: Vite HMR"
echo "  🔧 后端热更新: Wails Auto-rebuild"
echo "  🌐 按 Ctrl+C 停止"
echo ""

detect_webkit_tags

exec wails dev $WAILS_TAGS
