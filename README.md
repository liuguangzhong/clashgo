# ClashGo ⚡

**零 Rust 依赖。零子进程。100% Go。100% 全平台。**  
Mihomo 代理内核同进程嵌入，单二进制文件，开箱即用。

基于 [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev) 的 Go 全栈重写。

[快速开始](#-快速开始) | [架构文档](#-架构) | [常见问题](#-常见问题)

快速导航: [生产构建](#-生产构建) · [打包发布](#-打包发布) · [项目结构](#-项目结构) · [前端适配](#-前端适配说明)

全平台桌面代理客户端 — Go + Wails + React，一套代码，处处运行。

```
Mihomo 同进程嵌入 · Wails v2 WebView · React/MUI 前端 · 全平台支持
```

---

### 📢 与 Clash Verge Rev 的关系

ClashGo 是 Clash Verge Rev 的**架构重写**，保留了完整的前端 UI 和用户体验，后端从 Rust/Tauri 迁移到 Go/Wails：

| 原项目 | ClashGo |
|--------|---------|
| Rust + Tauri v2 | **Go + Wails v2** |
| Mihomo 外部子进程 | **Mihomo 库直接嵌入** (零 IPC) |
| clash-verge-service 守护进程 | **polkit / UAC 直接提权** |
| TypeScript `invoke()` IPC | **Wails `Call.ByName()` 绑定** |
| 前端源码 | **最大限度复用** (仅替换 IPC 层) |

---

### ✨ 特性

- ⚡ **零 IPC 延迟** — Mihomo 代理内核同进程运行，无 Socket/HTTP 通信开销
- 📦 **单二进制部署** — 一个 `clashgo` 可执行文件包含前后端一切
- 🏎️ **快速冷启动** — Go 编译，启动速度比 Electron/Tauri 更快
- 🔧 **Go 单一语言** — 后端 100% Go，无 Rust 工具链依赖
- 🌍 **全平台支持** — Windows / macOS (Intel + Apple Silicon) / Linux (x86_64, ARM64)
- 🖥️ **原生 WebView** — Windows: WebView2, Linux: WebKit2GTK, macOS: WKWebView
- 🛡️ **安全提权** — Linux polkit / Windows UAC，无常驻特权守护进程
- 🔄 **配置热加载** — 修改配置文件后同进程热重载 Mihomo 内核
- 📝 **JS 脚本引擎** — goja 引擎运行用户自定义配置增强脚本
- 🎨 **完整 UI** — 继承 Clash Verge Rev 全部界面（MUI + Monaco Editor + 国际化）

### 为什么选择 ClashGo

- **极简构建**：`go` + `node` + `wails` — 三个工具即可构建，无需 Rust 工具链
- **极低开销**：代理内核同进程嵌入，无序列化、无进程间通信
- **全平台一致**：一套代码编译 Windows / macOS / Linux，行为完全一致
- **易于贡献**：Go 语言门槛低于 Rust，社区贡献更方便
- **前端复用**：400+ 个 React 组件直接复用，UI 体验不打折

---

## 🚀 快速开始

### 第一步：安装基础依赖（手动）

以下工具**需要提前手动安装**，setup 脚本无法自动安装：

| 工具 | 版本要求 | 下载地址 |
|------|----------|----------|
| **Git** | — | [https://git-scm.com/downloads](https://git-scm.com/downloads) |
| **Go** | ≥ 1.22 | [https://go.dev/dl/](https://go.dev/dl/) |
| **Node.js** | ≥ 18 | [https://nodejs.org/](https://nodejs.org/) |

<details>
<summary><b>🪟 Windows 额外依赖</b></summary>

| 依赖 | 说明 | 下载地址 |
|------|------|----------|
| **WebView2 Runtime** | Windows 10/11 通常已内置 | [https://developer.microsoft.com/microsoft-edge/webview2/](https://developer.microsoft.com/microsoft-edge/webview2/) |
| **GCC** (CGo 编译需要) | 推荐 TDM-GCC | [https://jmeubank.github.io/tdm-gcc/](https://jmeubank.github.io/tdm-gcc/) |
| | 或 MSYS2 | [https://www.msys2.org/](https://www.msys2.org/) → `pacman -S mingw-w64-x86_64-gcc` |

</details>

<details>
<summary><b>🍎 macOS 额外依赖</b></summary>

```bash
# 安装 Xcode Command Line Tools
xcode-select --install
```

</details>

<details>
<summary><b>🐧 Linux (Ubuntu / Debian) 额外依赖</b></summary>

```bash
sudo apt-get update
sudo apt-get install -y \
  libwebkit2gtk-4.1-dev \
  libgtk-3-dev \
  libayatana-appindicator3-dev \
  build-essential \
  pkg-config
```

</details>

<details>
<summary><b>🐧 Linux (Fedora / RHEL) 额外依赖</b></summary>

```bash
sudo dnf install -y \
  webkit2gtk4.1-devel \
  gtk3-devel \
  libayatana-appindicator-gtk3-devel \
  gcc-c++ \
  pkgconfig
```

</details>

<details>
<summary><b>🐧 Linux (Arch) 额外依赖</b></summary>

```bash
sudo pacman -S webkit2gtk-4.1 gtk3 libayatana-appindicator pkgconf base-devel
```

</details>

### 第二步：克隆项目

```bash
git clone https://github.com/liuguangzhong/clashgo.git
cd clashgo
```

### 第三步：一键安装剩余依赖

运行 `setup` 脚本，会自动检查环境并安装以下依赖：

| 依赖 | 缺失时自动执行 |
|------|---------------|
| **pnpm** | `npm install -g pnpm` |
| **Wails CLI** | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2` |

同时会检查平台特有依赖（WebView2、GCC、webkit2gtk 等），缺失时给出安装提示。

```bash
# macOS / Linux
bash scripts/setup.sh

# Windows PowerShell
.\scripts\setup.ps1
```

> 验证环境是否就绪：`wails doctor`

### 第四步：一键编译

```bash
# macOS / Linux
bash scripts/build.sh

# Windows PowerShell
.\scripts\build.ps1
```

产物位于 `build/bin/`:

| 平台 | 产物 | 大小 |
|------|------|------|
| Windows | `clashgo.exe` | ~60MB |
| Linux | `clashgo` | ~50MB |
| macOS | `ClashGo.app` | ~60MB |

> 💡 二进制已内嵌 Mihomo 代理内核，无需额外下载。

### 一键启动（开发模式）

开发模式提供**前后端热更新**：前端修改即时反映，后端修改自动重编译。

```bash
# macOS / Linux
bash scripts/dev.sh

# Windows PowerShell
.\scripts\dev.ps1
```

### 或者直接用 Wails / Make

```bash
wails build      # 生产编译
wails dev        # 开发模式

make build        # 生产编译
make dev          # 开发模式
make test         # 运行测试
make clean        # 清理产物
```

---

## 📖 使用指南

ClashGo 的界面和操作方式**与 Clash Verge Rev 完全一致**。如果你用过原版，可以无缝上手。

### 启动

```bash
# 方式 1: 直接运行编译好的二进制
./build/bin/clashgo          # Linux/macOS
.\build\bin\clashgo.exe      # Windows

# 方式 2: 开发模式
wails dev
```

启动后会弹出主窗口，同时在系统托盘显示图标（右键可快速切换模式）。

### 添加订阅

1. 打开主界面，点击左侧 **「订阅」** (Profiles) 标签
2. 点击顶部 **「导入」** 按钮（或直接粘贴订阅链接到输入框）
3. 输入你的订阅 URL，点击 **「导入」**
4. 导入成功后，点击订阅卡片使其变为 **激活** 状态（高亮）
5. 代理核心会自动加载该配置并启动

> 💡 支持导入 Clash/Mihomo YAML 格式的订阅链接，也支持本地 YAML 文件。

### 配置系统代理

1. 在主界面左侧点击 **「设置」** (Settings)
2. 找到 **「系统代理」** 开关，打开即可自动设置系统代理
3. 系统代理会指向 Mihomo 的 mixed-port（默认 7897）

### 切换代理模式

在主界面顶部或系统托盘右键菜单中可切换三种模式：

| 模式 | 说明 |
|------|------|
| **Rule** (规则) | 按规则分流 — 国内直连，国外走代理（推荐日常使用） |
| **Global** (全局) | 所有流量走代理 |
| **Direct** (直连) | 所有流量直连，代理暂停 |

### 选择代理节点

1. 点击左侧 **「代理」** (Proxies) 标签
2. 展开代理组（如 PROXY / 自动选择 / 手动选择）
3. 点击想要的节点即可切换
4. 节点右侧的延迟数字可以点击测速

### 启用 TUN 模式（全局透明代理）

1. 进入 **「设置」** → 打开 **「TUN 模式」**
2. 首次开启需要管理员权限（Windows 弹 UAC，Linux 弹 polkit）
3. TUN 模式会接管系统所有网络流量，无需配置系统代理

> ⚠️ TUN 模式需要管理员/root 权限。

### 配置增强（Merge / Script）

1. 在 **「订阅」** 页面点击任意订阅卡片右侧的 **编辑** 按钮
2. 可以添加 **Merge 配置**（YAML 片段合并到订阅）或 **Script 脚本**（JS 动态修改）
3. 支持 Monaco Editor 代码编辑，语法高亮 + 自动补全

### 查看连接与日志

- **「连接」** (Connections) 页面：查看实时活跃连接、流量统计，可断开指定连接
- **「日志」** (Logs) 页面：查看 Mihomo 代理内核的实时日志

### 数据目录

| 平台 | 配置目录 |
|------|----------|
| Windows | `%APPDATA%\clashgo\` |
| macOS | `~/Library/Application Support/clashgo/` |
| Linux | `~/.config/clashgo/` |

配置文件包括：
- `verge.yaml` — 应用设置（语言、主题、系统代理、热键等）
- `clash.yaml` — Clash 基础配置（端口、DNS、模式等）
- `profiles.yaml` — 订阅列表
- `profiles/` 目录 — 订阅的 YAML 配置文件
- `runtime.yaml` — 增强流水线生成的实际运行配置

---

## 🏗️ 生产构建

```bash
# 当前平台
wails build

# 指定目标平台
wails build -platform linux/amd64
wails build -platform windows/amd64
wails build -platform darwin/amd64
wails build -platform darwin/arm64

# 跳过前端（仅重编译 Go）
wails build -s
```

---

## 📦 打包发布

```bash
# .deb (Ubuntu/Debian)
make package-deb

# .rpm (Fedora/RHEL)
make package-rpm

# Windows NSIS 安装包
wails build -nsis -platform windows/amd64

# macOS Universal Binary
wails build -platform darwin/universal
```

---

## 🧬 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                  Frontend (React + TypeScript)                    │
│  MUI · react-router · monaco-editor · i18n (20+ languages)      │
│  通信: window.go.api.ConfigAPI.Method() via Wails binding       │
└────────────────────────────┬────────────────────────────────────┘
                             │  WebView (WebKit2GTK / WebView2 / WKWebView)
┌────────────────────────────▼────────────────────────────────────┐
│                  Wails v2 Runtime (Go)                            │
│  main.go → App.Startup() → SystemTray + EventLoop               │
└────────────────────────────┬────────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────────┐
│                  API Binding Layer (api/*.go)                     │
│  ConfigAPI · ProfileAPI · ProxyAPI · SystemAPI                    │
│  BackupAPI · ServiceAPI · MediaUnlockAPI                          │
└──┬──────────────┬──────────────────────────┬────────────────────┘
   │              │                          │
┌──▼────┐  ┌──────▼──────────┐  ┌───────────▼────────────────────┐
│Config │  │ Core Manager    │  │ System Layer                    │
│Layer  │  │ Mihomo 生命周期  │  │ sysproxy (D-Bus/WinINet)       │
│       │  │ 增强流水线       │  │ TUN · hotkey · updater         │
│       │  │ 日志订阅        │  │ polkit / UAC                    │
└───────┘  └────────┬────────┘  └─────────────────────────────────┘
                    │
           ┌────────▼────────┐
           │ Mihomo Library  │
           │ (同进程嵌入)     │
           │ hub.Parse()     │
           │ tunnel.Tunnel   │
           └─────────────────┘
```

### 数据流: 配置修改 → 生效

```
用户界面操作
  → window.go.api.ConfigAPI.PatchVergeConfig({enable_tun_mode: true})
  → app.go → config.Manager.PatchVerge() → 写入 verge.yaml
  → core.Manager.UpdateConfig() → enhance.Engine.Run()
  → 生成 runtime.yaml → hub.Parse() (同进程热加载)
  → proxy.Guard.Refresh() → 前端事件: ConfigChanged
```

### 前端 → 后端通信路径

```
React 组件 → cmds.ts → wailsjs/go/api/ConfigAPI.js → Wails Runtime → api/config.go → internal/*
```

---

## 📁 项目结构

```
clashgo/
├── main.go                    # Wails 入口 (信号处理, 单例, GPU workaround)
├── app.go                     # App 结构体 (Startup/Shutdown/绑定)
├── wails.json                 # Wails 配置
├── Makefile                   # 构建/测试/打包
├── nfpm.yaml                  # Linux .deb/.rpm 打包
│
├── api/                       # 前端 API 绑定层
│   ├── config.go              #   Get/Patch VergeConfig, ClashConfig
│   ├── profiles.go            #   Import/Update/Delete Profile
│   ├── proxy.go               #   代理切换, 延迟测试
│   ├── system.go              #   系统代理, 网络接口
│   ├── backup.go              #   本地/WebDAV 备份
│   └── ...
│
├── internal/                  # 业务核心
│   ├── config/                #   配置管理 (IVerge, IClash, IProfiles)
│   ├── core/                  #   Mihomo 生命周期 + 增强流水线
│   ├── enhance/               #   YAML 合并, JS 脚本 (goja), 规则注入
│   ├── mihomo/                #   Mihomo REST/WS Client
│   ├── proxy/                 #   系统代理 (D-Bus/WinINet/gsettings)
│   ├── service/               #   特权操作 (polkit/UAC)
│   ├── tray/                  #   系统托盘
│   ├── hotkey/                #   全局热键
│   ├── updater/               #   自动更新
│   ├── backup/                #   备份管理
│   └── utils/                 #   工具集 (dirs, singleton, crypto)
│
├── frontend/                  # React 前端 (复用自 Clash Verge Rev)
│   ├── src/
│   │   ├── tauri-shim.ts      #   @tauri-apps/* → Wails/浏览器 兼容层
│   │   ├── tauri-plugin-mihomo-api.ts  # Mihomo API 直连 shim
│   │   ├── services/cmds.ts   #   IPC 调用 (Wails 绑定)
│   │   └── ...                #   400+ 业务组件
│   ├── wailsjs/               #   Wails 前端绑定 (auto-generated)
│   ├── vite.config.ts
│   └── tsconfig.json
│
└── scripts/                   # 一键脚本
    ├── build.sh / build.ps1   #   一键编译
    ├── dev.sh / dev.ps1       #   一键启动
    └── setup.sh / setup.ps1   #   环境检查 & 自动安装
```

---

## 🔌 前端适配说明

前端采用 **Shim 策略**，最大限度减少对原项目业务代码的改动：

| 文件 | 作用 |
|------|------|
| `src/tauri-shim.ts` | 替代全部 `@tauri-apps/*` 包 (window, event, clipboard, dialog, shell...) |
| `src/tauri-plugin-mihomo-api.ts` | 替代 `tauri-plugin-mihomo-api`，用 `fetch` + `WebSocket` 直连 Mihomo |
| `src/services/cmds.ts` | 300+ IPC 调用从 `invoke()` 映射到 Wails `Call.ByName()` 绑定 |
| `src/wails-runtime-shim.js` | Wails Runtime 桥接层 (导出 `Call` 对象) |

```
┌────────────────────────────┐      ┌──────────────────────────┐
│ 原 Tauri Import             │  →   │ ClashGo Shim              │
├────────────────────────────┤      ├──────────────────────────┤
│ @tauri-apps/api/*          │  →   │ tauri-shim.ts             │
│ @tauri-apps/plugin-*       │  →   │ tauri-shim.ts             │
│ tauri-plugin-mihomo-api    │  →   │ fetch + WebSocket 直连    │
│ invoke("cmd", args)        │  →   │ Call.ByName("api.X.Y")    │
└────────────────────────────┘      └──────────────────────────┘
```

---

## ❓ 常见问题

<details>
<summary><b><code>wails</code> 命令找不到？</b></summary>

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2

# 确认 GOPATH/bin 在 PATH 中
export PATH="$(go env GOPATH)/bin:$PATH"

# Windows PowerShell:
$env:PATH += ";$(go env GOPATH)\bin"
```

</details>

<details>
<summary><b>Linux 编译缺少 WebKit？</b></summary>

```bash
# Ubuntu/Debian
sudo apt-get install libwebkit2gtk-4.1-dev

# 一键诊断
wails doctor
```

</details>

<details>
<summary><b>Windows CGo 编译错误？</b></summary>

```
# "gcc: exec: ... not found" 错误
# 安装 TDM-GCC: https://jmeubank.github.io/tdm-gcc/
# 或 MSYS2 → pacman -S mingw-w64-x86_64-gcc
# 确保 gcc 在 PATH 中
```

</details>

<details>
<summary><b>macOS Apple Silicon 编译？</b></summary>

```bash
wails build -platform darwin/arm64      # ARM64
wails build -platform darwin/universal  # Universal Binary
```

</details>

<details>
<summary><b>只编译前端 / 只编译后端？</b></summary>

```bash
# 只编译前端
cd frontend && pnpm run build

# 只编译后端（跳过前端）
wails build -s
```

</details>

---

## 🛠 开发

```bash
wails dev                # 开发模式 (前后端热更新)
wails build              # 生产构建
go build ./...           # 仅编译 Go (快速检查)
go test -v ./...         # 运行测试
go vet ./...             # 代码检查
make lint                # golangci-lint
```

---

## 📄 License

GPL-3.0
