#!/bin/bash
# postinstall.sh - 安装后脚本
set -e

# 注册 .desktop 文件和 MIME 类型
if command -v update-desktop-database &>/dev/null; then
    update-desktop-database /usr/share/applications/
fi

# 注册 URL scheme handler
if command -v xdg-mime &>/dev/null; then
    xdg-mime default clashgo.desktop x-scheme-handler/clash
    xdg-mime default clashgo.desktop x-scheme-handler/clash-verge
fi

# 启用 systemd 服务（可选，用于 TUN 特权模式）
if systemctl is-system-running --quiet 2>/dev/null; then
    systemctl daemon-reload
    # 不自动 enable，由用户手动: sudo systemctl enable clashgo-service
fi

echo "ClashGo installed successfully."
echo "Run: clashgo"
