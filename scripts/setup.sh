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
        if go install github.com/wailsapp/wails/v2/cmd/wails@v2.11.0 2>&1; then
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

# 辅助函数：检查 Linux 开发库是否安装
# 优先 pkg-config，备用 dpkg/rpm，再备用 ldconfig
check_linux_dev_lib() {
    local pkg_name="$1"       # pkg-config 名称，如 webkit2gtk-4.1
    local deb_dev="$2"        # Debian dev 包名，如 libwebkit2gtk-4.1-dev
    local deb_runtime="$3"    # Debian 运行时包名，如 libwebkit2gtk-4.1-0
    local rpm_dev="$4"        # RPM dev 包名
    local arch_pkg="$5"       # Arch 包名
    local so_pattern="$6"     # 共享库模式，如 libwebkit2gtk-4.1

    # 方式1: pkg-config (最准确，检测 -dev 包)
    if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists "$pkg_name" 2>/dev/null; then
        ok "$pkg_name (dev)"
        return 0
    fi

    # 方式2: dpkg 检测 (Debian/Ubuntu)
    if command -v dpkg >/dev/null 2>&1; then
        if dpkg -s "$deb_dev" >/dev/null 2>&1; then
            ok "$pkg_name (dev) [dpkg]"
            return 0
        elif dpkg -s "$deb_runtime" >/dev/null 2>&1; then
            # 运行时包已装，但缺少开发包
            warn "$pkg_name: 运行时包 $deb_runtime 已安装，但编译需要开发包"
            echo "    sudo apt install $deb_dev"
            ERRORS=$((ERRORS+1))
            return 1
        fi
    fi

    # 方式3: rpm 检测 (Fedora/RHEL)
    if command -v rpm >/dev/null 2>&1; then
        if rpm -q "$rpm_dev" >/dev/null 2>&1; then
            ok "$pkg_name (dev) [rpm]"
            return 0
        fi
    fi

    # 方式4: ldconfig 检测共享库是否存在 (最后手段)
    if ldconfig -p 2>/dev/null | grep -q "$so_pattern"; then
        warn "$pkg_name: 共享库存在，但未检测到开发包 (头文件/.pc文件)"
        echo "    Ubuntu/Debian: sudo apt install $deb_dev"
        echo "    Fedora:        sudo dnf install $rpm_dev"
        echo "    Arch:          sudo pacman -S $arch_pkg"
        ERRORS=$((ERRORS+1))
        return 1
    fi

    # 完全没有
    fail "$pkg_name 未安装"
    echo "    Ubuntu/Debian: sudo apt install $deb_dev"
    echo "    Fedora:        sudo dnf install $rpm_dev"
    echo "    Arch:          sudo pacman -S $arch_pkg"
    ERRORS=$((ERRORS+1))
    return 1
}

OS="$(uname -s)"
case "$OS" in
    Linux)
        info "检查 Linux 开发库..."

        # 检测发行版和包管理器
        DISTRO=""
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            DISTRO="$ID"
        fi

        # ── Debian/Ubuntu 系 ──
        if command -v apt-get >/dev/null 2>&1; then
            NEED_INSTALL=()

            # pkg-config
            command -v pkg-config >/dev/null 2>&1 || NEED_INSTALL+=("pkg-config")

            # build-essential (gcc 等)
            command -v gcc >/dev/null 2>&1 || NEED_INSTALL+=("build-essential")

            # webkit2gtk-4.1 dev (优先4.1，fallback 4.0)
            if ! dpkg -s libwebkit2gtk-4.1-dev >/dev/null 2>&1 && ! dpkg -s libwebkit2gtk-4.0-dev >/dev/null 2>&1; then
                NEED_INSTALL+=("libwebkit2gtk-4.1-dev")
            fi

            # libsoup-3.0 (webkit2gtk-4.1 需要)
            if ! dpkg -s libsoup-3.0-dev >/dev/null 2>&1; then
                NEED_INSTALL+=("libsoup-3.0-dev")
            fi

            # GTK 3 dev
            if ! dpkg -s libgtk-3-dev >/dev/null 2>&1; then
                NEED_INSTALL+=("libgtk-3-dev")
            fi

            # libayatana-appindicator (系统托盘)
            if ! dpkg -s libayatana-appindicator3-dev >/dev/null 2>&1; then
                NEED_INSTALL+=("libayatana-appindicator3-dev")
            fi

            if [ ${#NEED_INSTALL[@]} -gt 0 ]; then
                warn "缺少以下开发库，正在自动安装..."
                echo "    ${NEED_INSTALL[*]}"
                echo ""
                if sudo apt-get update -qq && sudo apt-get install -y "${NEED_INSTALL[@]}"; then
                    ok "Linux 开发库安装完成"
                else
                    fail "部分开发库安装失败，请手动运行:"
                    echo "    sudo apt-get install -y ${NEED_INSTALL[*]}"
                    ERRORS=$((ERRORS+1))
                fi
            else
                ok "Linux 开发库已就绪"
            fi

        # ── Fedora/RHEL 系 ──
        elif command -v dnf >/dev/null 2>&1; then
            NEED_INSTALL=()

            command -v pkg-config >/dev/null 2>&1 || NEED_INSTALL+=("pkgconfig")
            command -v gcc >/dev/null 2>&1 || NEED_INSTALL+=("gcc-c++")

            if ! rpm -q webkit2gtk4.1-devel >/dev/null 2>&1; then
                NEED_INSTALL+=("webkit2gtk4.1-devel")
            fi
            if ! rpm -q gtk3-devel >/dev/null 2>&1; then
                NEED_INSTALL+=("gtk3-devel")
            fi
            if ! rpm -q libayatana-appindicator-gtk3-devel >/dev/null 2>&1; then
                NEED_INSTALL+=("libayatana-appindicator-gtk3-devel")
            fi

            if [ ${#NEED_INSTALL[@]} -gt 0 ]; then
                warn "缺少以下开发库，正在自动安装..."
                echo "    ${NEED_INSTALL[*]}"
                if sudo dnf install -y "${NEED_INSTALL[@]}"; then
                    ok "Linux 开发库安装完成"
                else
                    fail "部分开发库安装失败"
                    ERRORS=$((ERRORS+1))
                fi
            else
                ok "Linux 开发库已就绪"
            fi

        # ── Arch 系 ──
        elif command -v pacman >/dev/null 2>&1; then
            NEED_INSTALL=()

            pacman -Qi pkgconf >/dev/null 2>&1 || NEED_INSTALL+=("pkgconf")
            pacman -Qi base-devel >/dev/null 2>&1 || NEED_INSTALL+=("base-devel")
            pacman -Qi webkit2gtk-4.1 >/dev/null 2>&1 || NEED_INSTALL+=("webkit2gtk-4.1")
            pacman -Qi gtk3 >/dev/null 2>&1 || NEED_INSTALL+=("gtk3")
            pacman -Qi libayatana-appindicator >/dev/null 2>&1 || NEED_INSTALL+=("libayatana-appindicator")

            if [ ${#NEED_INSTALL[@]} -gt 0 ]; then
                warn "缺少以下开发库，正在自动安装..."
                echo "    ${NEED_INSTALL[*]}"
                if sudo pacman -S --noconfirm "${NEED_INSTALL[@]}"; then
                    ok "Linux 开发库安装完成"
                else
                    fail "部分开发库安装失败"
                    ERRORS=$((ERRORS+1))
                fi
            else
                ok "Linux 开发库已就绪"
            fi

        else
            fail "未识别的 Linux 发行版，请手动安装以下开发库:"
            echo "    webkit2gtk-4.1 (或 4.0), gtk3, libayatana-appindicator3, pkg-config, gcc"
            ERRORS=$((ERRORS+1))
        fi
        ;;
    Darwin)
        if xcode-select -p >/dev/null 2>&1; then
            ok "Xcode CLT: $(xcode-select -p)"
        else
            warn "Xcode Command Line Tools 未安装，正在安装..."
            xcode-select --install 2>/dev/null || {
                fail "请手动运行: xcode-select --install"
                ERRORS=$((ERRORS+1))
            }
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
