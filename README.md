# ClashGo

> **Clash Verge Rev 的 Go 全栈重写版本**  
> Go + Wails + React — 全平台桌面代理客户端

---

## 目录

- [项目简介](#项目简介)
- [架构概览](#架构概览)
- [环境要求](#环境要求)
- [一键编译](#一键编译)
- [一键启动（开发模式）](#一键启动开发模式)
- [生产构建](#生产构建)
- [打包发布](#打包发布)
- [项目结构](#项目结构)
- [前端适配说明](#前端适配说明)
- [常见问题](#常见问题)

---

## 项目简介

ClashGo 是 [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev) 的 Go 重写版。

**核心目标：**

| 目标 | 效果 |
|------|------|
| ❌ Rust/Tauri 依赖 → ✅ Go 单一语言 | 构建工具链更简单 |
| ❌ Mihomo 外部子进程 → ✅ Go 库直接嵌入 | 零 IPC、零序列化开销 |
| ❌ clash-verge-service 守护进程 → ✅ polkit / UAC 直接提权 | 减少攻击面 |
| ✅ 前端 React/MUI 代码最大限度复用 | 仅替换 IPC 层 |

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────┐
│                     前端层 (React + TypeScript)                      │
│  MUI + react-router + monaco-editor + react-hook-form               │
│  通信: window.go.XXX.Method() via Wails binding                     │
└───────────────────────────────┬─────────────────────────────────────┘
                                │  WebView
┌───────────────────────────────▼─────────────────────────────────────┐
│                    Wails v2 GUI Layer (Go)                           │
│  main.go → App.Startup() → 系统托盘 + 事件循环                      │
└───────────────────────────────┬─────────────────────────────────────┘
                                │  方法绑定
┌───────────────────────────────▼─────────────────────────────────────┐
│                    API 绑定层 (api/*.go)                             │
│  ConfigAPI · ProfileAPI · ProxyAPI · SystemAPI                       │
│  BackupAPI · ServiceAPI · MediaUnlockAPI                             │
└──┬────────────────┬────────────────────────────────┬────────────────┘
   │                │                                │
┌──▼──────┐  ┌──────▼───────────────┐  ┌────────────▼──────────────┐
│ Config  │  │   Core Manager       │  │   System Layer            │
│ Layer   │  │   Mihomo 生命周期     │  │   sysproxy · tun · hotkey │
│         │  │   配置增强流水线      │  │   polkit / UAC 提权       │
└─────────┘  │   日志流订阅         │  └───────────────────────────┘
             └──────────┬──────────┘
                        │
               ┌────────▼──────────┐
               │  Mihomo Library   │
               │ (同进程 Go 库)     │
               └───────────────────┘
```

**前端 ↔ 后端通信路径：**

```
React组件 → cmds.ts → wailsjs/go/api/ConfigAPI.js → Wails Runtime → api/config.go
```

---

## 环境要求

### 所有平台通用

| 工具 | 版本 | 安装方式 |
|------|------|----------|
| **Go** | ≥ 1.22 | https://go.dev/dl/ |
| **Node.js** | ≥ 18 | https://nodejs.org/ |
| **pnpm** | ≥ 8 | `npm install -g pnpm` |
| **Wails CLI** | v2.9.x | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2` |

### Windows 额外依赖

```powershell
# WebView2 (Windows 10+ 已内置)
# 如果没有，通过 https://developer.microsoft.com/microsoft-edge/webview2/ 安装

# 可选: GCC (CGo 编译，部分依赖需要)
# 推荐安装 https://www.msys2.org/ 后添加 GCC 到 PATH
# 或使用 https://jmeubank.github.io/tdm-gcc/
```

### macOS 额外依赖

```bash
# Xcode Command Line Tools (含 clang)
xcode-select --install
```

### Linux (Ubuntu/Debian) 额外依赖

```bash
# WebKit2GTK + 必要开发库
sudo apt-get update
sudo apt-get install -y \
  libwebkit2gtk-4.1-dev \
  libgtk-3-dev \
  libayatana-appindicator3-dev \
  build-essential \
  pkg-config
```

### Linux (Fedora/RHEL)

```bash
sudo dnf install -y \
  webkit2gtk4.1-devel \
  gtk3-devel \
  libayatana-appindicator-gtk3-devel \
  gcc-c++ \
  pkgconfig
```

### Linux (Arch)

```bash
sudo pacman -S webkit2gtk-4.1 gtk3 libayatana-appindicator pkgconf base-devel
```

---

## 一键编译

### Windows (PowerShell)

```powershell
# 方式一：使用一键脚本
.\scripts\build.ps1

# 方式二：手动
cd clashgo
pnpm install --prefix frontend
wails build
# 产物: build\bin\clashgo.exe
```

### macOS / Linux (Bash)

```bash
# 方式一：使用一键脚本
chmod +x scripts/build.sh
./scripts/build.sh

# 方式二：手动
cd clashgo
pnpm install --prefix frontend
wails build
# 产物: build/bin/clashgo (Linux) 或 build/bin/ClashGo.app (macOS)
```

### 使用 Makefile

```bash
make build          # 当前平台生产构建
make build-linux    # 交叉编译 Linux amd64
make build-windows  # 交叉编译 Windows amd64
```

---

## 一键启动（开发模式）

开发模式提供 **前后端热更新**：前端修改即时反映，后端修改自动重编译。

### Windows (PowerShell)

```powershell
# 方式一：使用一键脚本
.\scripts\dev.ps1

# 方式二：手动
cd clashgo
wails dev
```

### macOS / Linux (Bash)

```bash
# 方式一：使用一键脚本
chmod +x scripts/dev.sh
./scripts/dev.sh

# 方式二：手动
cd clashgo
wails dev
```

### 使用 Makefile

```bash
make dev
```

> **提示**：`wails dev` 会自动执行 `pnpm install` + `pnpm run dev`（前端开发服务器），  
> 然后编译并启动 Go 后端，两者通过 WebSocket 桥接实现热更新。

---

## 生产构建

### 全平台构建命令

```bash
# 当前平台
wails build

# 指定目标平台
wails build -platform linux/amd64
wails build -platform windows/amd64
wails build -platform darwin/amd64
wails build -platform darwin/arm64

# 带版本号
wails build -ldflags "-X main.Version=v1.0.0 -s -w"

# 跳过前端构建（仅重编译 Go）
wails build -s
```

### 构建产物

| 平台 | 产物路径 | 大小预估 |
|------|----------|---------|
| Windows | `build/bin/clashgo.exe` | ~30MB |
| Linux | `build/bin/clashgo` | ~25MB |
| macOS | `build/bin/ClashGo.app` | ~35MB |

---

## 打包发布

### Linux .deb 包

```bash
# 需要 nfpm: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
make package-deb
# 产物: dist/clashgo_1.0.0_amd64.deb
```

### Linux .rpm 包

```bash
make package-rpm
# 产物: dist/clashgo-1.0.0.x86_64.rpm
```

### Windows NSIS 安装包

```powershell
# Wails 内建支持
wails build -nsis -platform windows/amd64
# 产物: build/bin/clashgo-amd64-installer.exe
```

### macOS .dmg

```bash
# Wails 内建支持
wails build -platform darwin/universal
# 然后用 create-dmg 或 hdiutil 打包 .app
```

---

## 项目结构

```
clashgo/
├── main.go                          # Wails 应用入口（信号处理、单例、GPU workaround）
├── app.go                           # App 主结构体（Startup/Shutdown/绑定注册）
├── wails.json                       # Wails 项目配置
├── Makefile                         # 构建、测试、打包命令
├── go.mod / go.sum                  # Go 模块依赖
├── nfpm.yaml                        # Linux .deb/.rpm 打包配置
│
├── api/                             # 前端 API 绑定层
│   ├── config.go                    # 配置：Get/Patch VergeConfig, ClashConfig
│   ├── config_extra.go              # 配置附加：GetClashInfo, GetRuntimeYaml
│   ├── profiles.go                  # 订阅管理：Import/Update/Delete Profile
│   ├── proxy.go                     # 代理控制：切换模式、延迟测试
│   ├── system.go                    # 系统操作：系统代理、网络接口
│   ├── backup.go                    # 备份管理：本地/WebDAV
│   ├── service.go                   # 特权服务管理
│   ├── mediaunlock.go               # 流媒体解锁检测
│   └── http.go                      # Mihomo HTTP API 代理
│
├── internal/
│   ├── config/                      # 配置管理
│   │   ├── types.go                 # IVerge, IClash, IProfiles 类型
│   │   ├── verge.go                 # Verge 应用配置
│   │   ├── clash.go                 # Clash 基础配置
│   │   ├── profiles.go              # 订阅配置
│   │   ├── runtime.go               # 运行时配置生成
│   │   └── manager.go               # 全局配置管理器
│   │
│   ├── core/                        # Mihomo 核心管理
│   │   ├── manager.go               # CoreManager 单例，生命周期
│   │   ├── enhancer.go              # 配置增强流水线
│   │   └── log.go                   # 日志订阅与转发
│   │
│   ├── enhance/                     # 配置增强引擎
│   │   ├── engine.go                # 主流水线入口
│   │   ├── merge.go                 # YAML 深度合并
│   │   ├── script.go                # JavaScript 脚本（goja）
│   │   └── ...
│   │
│   ├── mihomo/                      # Mihomo HTTP Client
│   │   └── client.go                # REST + WebSocket 封装
│   │
│   ├── proxy/                       # 系统代理控制
│   │   ├── linux.go                 # D-Bus / gsettings
│   │   ├── windows.go               # WinINet
│   │   └── guard.go                 # 代理守卫
│   │
│   ├── service/                     # 特权操作
│   ├── tray/                        # 系统托盘
│   ├── hotkey/                      # 全局热键
│   ├── updater/                     # 自动更新
│   ├── backup/                      # 备份
│   ├── notify/                      # 系统通知
│   └── utils/                       # 工具集
│
├── frontend/                        # React 前端
│   ├── src/
│   │   ├── services/cmds.ts         # 后端 API 调用（Wails 绑定）
│   │   ├── tauri-shim.ts            # Tauri → Wails/浏览器 兼容层
│   │   ├── tauri-plugin-mihomo-api.ts # Mihomo API 直连 shim
│   │   └── ...                      # 业务组件（与原项目一致）
│   │
│   ├── wailsjs/                     # Wails 前端绑定
│   │   ├── go/api/*.js              # Go API 绑定（ConfigAPI, ProfileAPI...）
│   │   ├── go/main/App.js           # App 绑定
│   │   └── runtime/runtime.js       # Wails Runtime stub
│   │
│   ├── vite.config.ts               # Vite 构建配置（含别名映射）
│   ├── tsconfig.json                # TypeScript 配置
│   └── package.json
│
├── scripts/                         # 一键编译/启动脚本
│   ├── build.sh                     # Unix 一键编译
│   ├── build.ps1                    # Windows 一键编译
│   ├── dev.sh                       # Unix 一键启动
│   ├── dev.ps1                      # Windows 一键启动
│   └── setup.sh                     # 环境检查 & 依赖安装
│
└── build/                           # Wails 构建配置 & 产物
    └── bin/                         # 编译输出目录
```

---

## 前端适配说明

前端从原项目（Tauri）迁移到 Wails，采用 **Shim 策略**，最大限度减少业务代码改动。

### 适配层文件

| 文件 | 作用 | 行数 |
|------|------|------|
| `src/tauri-shim.ts` | 替代所有 `@tauri-apps/*` 包 | ~280 |
| `src/tauri-plugin-mihomo-api.ts` | 替代 `tauri-plugin-mihomo-api` | ~360 |
| `src/services/cmds.ts` | IPC 调用从 `invoke()` → Wails 绑定 | ~260 |
| `src/providers/window/window-provider.tsx` | 窗口控制 | ~110 |

### 映射关系

```
┌──────────────────────────────┐      ┌────────────────────────┐
│ 原 Tauri Import              │  →   │ ClashGo Shim            │
├──────────────────────────────┤      ├────────────────────────┤
│ @tauri-apps/api/core         │  →   │ tauri-shim.ts           │
│ @tauri-apps/api/event        │  →   │ tauri-shim.ts           │
│ @tauri-apps/api/window       │  →   │ tauri-shim.ts           │
│ @tauri-apps/plugin-http      │  →   │ globalThis.fetch        │
│ @tauri-apps/plugin-clipboard │  →   │ navigator.clipboard     │
│ @tauri-apps/plugin-dialog    │  →   │ stub → Go backend       │
│ @tauri-apps/plugin-shell     │  →   │ window.open()           │
│ @tauri-apps/plugin-updater   │  →   │ stub → Go updater       │
│ tauri-plugin-mihomo-api      │  →   │ fetch + WebSocket 直连  │
│ invoke("cmd", args)          │  →   │ Wails Call.ByName()     │
└──────────────────────────────┘      └────────────────────────┘
```

---

## 常见问题

### Q: `wails` 命令找不到？

```bash
# 确认 GOPATH/bin 在 PATH 中
go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2

# 检查
echo $GOPATH  # 或 go env GOPATH
# 确保 $GOPATH/bin 在 $PATH 中

# Windows PowerShell:
$env:PATH += ";$(go env GOPATH)\bin"
```

### Q: Linux 编译缺少 WebKit？

```bash
# Ubuntu/Debian
sudo apt-get install libwebkit2gtk-4.1-dev

# 检查环境
wails doctor
```

### Q: Windows CGo 编译错误？

```
# 如果看到 "gcc: exec: ... not found" 错误
# 需要安装 GCC for Windows：
# 1. TDM-GCC: https://jmeubank.github.io/tdm-gcc/
# 2. 或 MSYS2: https://www.msys2.org/ → pacman -S mingw-w64-x86_64-gcc
# 然后确保 gcc 在 PATH 中
```

### Q: macOS Apple Silicon 编译？

```bash
# 原生 ARM64
wails build -platform darwin/arm64

# Universal Binary (同时支持 Intel + M1)
wails build -platform darwin/universal
```

### Q: 前端 pnpm install 失败？

```bash
cd frontend
pnpm install --no-frozen-lockfile
```

### Q: 如何只编译前端不启动后端？

```bash
cd frontend
pnpm run build
# 产物在 frontend/dist/
```

### Q: 如何只编译后端不重建前端？

```bash
wails build -s  # -s = skip frontend build
```

---

## License

GPL-3.0
