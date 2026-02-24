# ClashGo 架构文档

> 版本: 1.0.0  
> 日期: 2026-02-24  
> 目标平台: Ubuntu 22.04 LTS (linux/amd64)，兼容 Windows / macOS  

---

## 一、项目定位

ClashGo 是 Clash Verge Rev 的 Go 全栈重写版本。

**核心目标：**
1. 消除 Rust/Tauri 依赖，统一为 Go 单一语言
2. 将 Mihomo 代理内核从**外部子进程**变为**Go 库直接嵌入**（同进程，零 IPC 开销）
3. 消除 `clash-verge-service` 特权守护进程（Linux polkit 直接替代）
4. 原生适配 Ubuntu 22.04（WebKit2GTK + D-Bus + systemd）
5. 前端 React/TypeScript 代码最大限度复用

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                     前端层 (React + TypeScript)                      │
│  组件: MUI + react-router + monaco-editor + react-hook-form          │
│  通信: window.go.XXX.Method() via Wails binding (替换 Tauri invoke) │
└───────────────────────────────┬─────────────────────────────────────┘
                                │  WebView (WebKit2GTK on Linux)
┌───────────────────────────────▼─────────────────────────────────────┐
│                    Wails v2 GUI Layer (Go)                           │
│  职责: WebView 宿主 / 前后端绑定 / 系统托盘 / 事件循环              │
│  文件: main.go, app.go                                               │
└───────────────────────────────┬─────────────────────────────────────┘
                                │  方法调用
┌───────────────────────────────▼─────────────────────────────────────┐
│                    API 绑定层 (Go)                                   │
│  api/config.go    - 配置相关命令 (GetVergeConfig, PatchVergeConfig)  │
│  api/proxy.go     - 代理操作 (GetClashInfo, PatchClashMode)          │
│  api/profiles.go  - 订阅管理 (ImportProfile, UpdateProfile)          │
│  api/system.go    - 系统操作 (GetSysProxy, GetNetworkInterfaces)     │
│  api/backup.go    - 备份管理 (CreateLocalBackup, SaveWebDAVConfig)   │
└──┬────────────────┬────────────────────────────────────┬────────────┘
   │                │                                    │
┌──▼──────┐  ┌──────▼──────────────────────┐  ┌────────▼───────────┐
│ Config  │  │     Core Manager             │  │   System Layer     │
│ Layer   │  │  internal/core/manager.go    │  │                    │
│         │  │  - Mihomo 生命周期管理        │  │ proxy/linux.go     │
│ verge   │  │  - 配置热加载                 │  │ - gsettings D-Bus  │
│ clash   │  │  - 日志流订阅                 │  │ - 系统代理守卫     │
│ profile │  │                              │  │                    │
│ runtime │  │  internal/core/enhancer.go   │  │ proxy/tun.go       │
│         │  │  - 增强流水线 (enhance/)      │  │ - nftables/TUN     │
└──┬──────┘  └──────────────┬───────────────┘  │ - polkit 提权      │
   │                        │                  └────────────────────┘
   │              ┌─────────▼──────────┐
   │              │   Mihomo Library   │
   │              │  (同进程嵌入)       │
   │              │                    │
   │              │  hub.LoadConfig()  │
   │              │  tunnel.Tunnel     │
   │              │  dns.DefaultResolver│
   └──────────────► config.Parse()    │
                  └────────────────────┘
```

---

## 三、目录结构

```
clashgo/
├── main.go                          # Wails 应用入口
├── app.go                           # Wails App 主结构体，绑定前端
├── go.mod
├── go.sum
├── wails.json                       # Wails 项目配置
│
├── api/                             # 前端可调用的 API 绑定层
│   ├── config.go                    # 配置相关命令
│   ├── proxy.go                     # 代理控制命令
│   ├── profiles.go                  # 订阅管理命令
│   ├── system.go                    # 系统信息/操作命令
│   └── backup.go                    # 备份管理命令
│
├── internal/
│   ├── config/                      # 配置管理（对应原 src-tauri/src/config/）
│   │   ├── types.go                 # IVerge, IClash, IProfiles 类型定义
│   │   ├── verge.go                 # Verge 应用配置读写
│   │   ├── clash.go                 # Clash 基础配置管理
│   │   ├── profiles.go              # 订阅配置管理
│   │   ├── runtime.go               # 运行时配置生成
│   │   └── manager.go               # Config 全局管理器（单例）
│   │
│   ├── core/                        # 代理核心管理（对应原 src-tauri/src/core/manager/）
│   │   ├── manager.go               # CoreManager 单例，Mihomo 生命周期
│   │   ├── enhancer.go              # 调用 enhance 流水线并应用配置
│   │   └── log.go                   # Mihomo 日志订阅与转发
│   │
│   ├── enhance/                     # 配置增强流水线（对应原 src-tauri/src/enhance/）
│   │   ├── engine.go                # 主流水线入口 Enhance()
│   │   ├── merge.go                 # Merge 策略（YAML 深度合并）
│   │   ├── script.go                # JavaScript 脚本执行（goja 引擎）
│   │   ├── rules.go                 # Rules/Proxies/Groups 注入
│   │   ├── tun.go                   # TUN 配置注入
│   │   └── field.go                 # 配置字段排序与过滤
│   │
│   ├── proxy/                       # 系统代理控制（对应原 src-tauri/src/core/sysopt.rs）
│   │   ├── sysproxy.go              # 接口定义（跨平台）
│   │   ├── linux.go                 # Linux D-Bus / gsettings 实现
│   │   ├── windows.go               # Windows WinINet 实现
│   │   └── guard.go                 # 代理守卫（定期校验代理设置）
│   │
│   ├── service/                     # 特权操作（替代原 clash-verge-service）
│   │   ├── service.go               # 接口定义
│   │   ├── linux.go                 # Linux polkit / systemd 集成
│   │   └── windows.go               # Windows 服务管理
│   │
│   ├── tray/                        # 系统托盘（对应原 src-tauri/src/core/tray/）
│   │   └── tray.go                  # Wails 托盘 + 菜单动态更新
│   │
│   ├── hotkey/                      # 全局热键（对应原 src-tauri/src/core/hotkey.rs）
│   │   └── hotkey.go
│   │
│   ├── updater/                     # 自动更新（对应原 tauri-plugin-updater）
│   │   └── updater.go
│   │
│   ├── backup/                      # 备份（对应原 src-tauri/src/core/backup.rs）
│   │   ├── local.go                 # 本地备份
│   │   └── webdav.go                # WebDAV 远程备份
│   │
│   ├── notify/                      # 系统通知
│   │   └── notify.go
│   │
│   └── utils/
│       ├── dirs.go                  # 目录路径管理（对应原 utils/dirs.rs）
│       ├── singleton.go             # 单例进程检测
│       ├── help.go                  # YAML 读写/加密工具
│       ├── network.go               # 网络接口枚举
│       └── init.go                  # 应用初始化序列
│
└── frontend/                        # 从现有项目适配的 React 前端
    ├── src/                         # 改动最小：仅替换 @tauri-apps/api 调用
    └── ...
```

---

## 四、关键设计决策

### 4.1 Mihomo 嵌入策略

```go
// 不再启动子进程，直接 import Mihomo 包
import (
    "github.com/MetaCubeX/mihomo/hub"
    "github.com/MetaCubeX/mihomo/hub/executor"
    mihomoLog "github.com/MetaCubeX/mihomo/log"
    "github.com/MetaCubeX/mihomo/constant"
)

// 加载配置（替代原来的 Unix Socket reload 命令）
func (m *CoreManager) ApplyConfig(configPath string) error {
    return hub.Parse(configPath)  // 直接调用，同进程
}
```

**优势：**
- 零 IPC 延迟（原来通过 Unix Socket 通信）
- 消除 `clash-verge-service` 特权进程
- 错误直接以 Go error 返回，无需 HTTP 状态码解析
- 内存共享，无序列化开销

### 4.2 配置增强流水线

```
原始订阅 YAML
    → Global Merge（全局合并配置）
    → Global Script（全局 JS 脚本, goja 执行）
    → Profile Rules 注入
    → Profile Proxies 注入
    → Profile Merge
    → Profile Script
    → 合并 Clash 基础配置
    → 应用内置脚本（builtin scripts）
    → TUN 配置注入
    → 字段排序
    → DNS 配置注入
    → 写入 runtime.yaml
    → hub.Parse(runtime.yaml)   ← 同进程热加载
```

### 4.3 Linux 系统代理

```go
// 通过 D-Bus 调用 gsettings（不依赖 sysproxy-rs）
type LinuxProxy struct {
    conn *dbus.Conn
}

func (p *LinuxProxy) SetHTTPProxy(host string, port int) error {
    // 通过 org.gnome.system.proxy D-Bus 接口设置
    obj := p.conn.Object("org.gnome.system.proxy", "/org/gnome/system/proxy")
    // 设置 host、port、mode
}

// 同时支持 KDE / 环境变量 fallback
```

### 4.4 JS 脚本引擎（goja）

```go
import "github.com/dop251/goja"

func ExecuteScript(script string, config map[string]any, profileName string) (map[string]any, error) {
    vm := goja.New()
    vm.Set("__profile_name", profileName)
    
    // 注入 console.log
    vm.Set("console", map[string]any{
        "log": func(args ...any) { /* 收集日志 */ },
    })
    
    // 执行用户脚本
    _, err := vm.RunString(script)
    // 调用 main(params) 函数
    main, ok := goja.AssertFunction(vm.Get("main"))
    result, err := main(goja.Undefined(), vm.ToValue(config))
    // 返回修改后的配置
}
```

### 4.5 Wails 前端绑定

```go
// app.go - 前端可直接调用
type App struct {
    config  *api.ConfigAPI
    proxy   *api.ProxyAPI
    profile *api.ProfileAPI
    system  *api.SystemAPI
}

// 对应前端: window.go.App.GetVergeConfig()
func (a *App) GetVergeConfig() (*config.IVerge, error) {
    return a.config.GetVergeConfig()
}

// 对应前端: window.go.App.PatchVergeConfig(patch)
func (a *App) PatchVergeConfig(patch config.IVerge) error {
    return a.config.PatchVergeConfig(patch)
}
```

---

## 五、并发模型

```
主 goroutine (Wails 事件循环)
│
├── ProxyGuard goroutine       - 定期检查系统代理是否被篡改
│
├── AutoUpdate goroutine       - 定期检查版本更新（cron）
│
├── AutoBackup goroutine       - 定期备份配置（cron）
│
├── LogStreamer goroutine       - 订阅 Mihomo 日志，推送到前端
│
├── ProfileUpdater goroutine   - 订阅到期，自动更新所有配置
│
├── SignalHandler goroutine    - SIGTERM/SIGINT 优雅退出
│
└── Mihomo 内部 goroutines     - DNS / Tunnel / Inbound / Outbound
                                 (由 Mihomo 库自行管理)
```

---

## 六、Linux 平台特有处理

| 功能 | 实现方案 |
|---|---|
| 系统代理 | D-Bus `org.gnome.system.proxy` + KDE fallback |
| 系统托盘 | `libayatana-appindicator` via Wails 内置托盘 |
| TUN 模式 | `/dev/tun` + polkit 提权 + nftables 规则注入 |
| 自启动 | `~/.config/autostart/clashgo.desktop` |
| 深度链接 | `xdg-mime` + `.desktop` schema handler |
| 单例检测 | Unix Domain Socket（`/tmp/clashgo.lock`）|
| URL Scheme | `clash://` / `clash-verge://` xdg 注册 |
| NVIDIA 兼容 | `WEBKIT_DISABLE_DMABUF_RENDERER=1` 自动检测 |
| Wayland 支持 | `GDK_BACKEND=x11` fallback 或原生 Wayland |

---

## 七、数据流：配置修改 → 生效

```
用户修改配置
    │
    ▼
frontend: window.go.App.PatchVergeConfig({enable_tun_mode: true})
    │
    ▼
app.go: App.PatchVergeConfig() → config.Manager.PatchVerge()
    │ 写入 verge.yaml
    ▼
core.Manager.UpdateConfig()
    │ 触发增强流水线
    ▼
enhance.Engine.Run()  →  生成 runtime.yaml
    │
    ▼
hub.Parse(runtime.yaml)  ← Mihomo 同进程热加载
    │
    ▼
proxy.Guard.Refresh()  ← 更新系统代理设置
    │
    ▼
前端收到事件: ConfigChanged → UI 更新
```

---

## 八、依赖清单

```
github.com/wailsapp/wails/v2              # GUI 框架
github.com/MetaCubeX/mihomo              # 代理内核（库）
github.com/dop251/goja                   # JavaScript 引擎
github.com/godbus/dbus/v5                # Linux D-Bus（系统代理）
github.com/robfig/cron/v3                # 定时任务
github.com/emersion/go-webdav            # WebDAV 备份
github.com/shirou/gopsutil/v3            # 系统信息
gopkg.in/yaml.v3                         # YAML 解析
golang.org/x/crypto                      # AES-GCM 加密
github.com/getlantern/systray            # 系统托盘 (Wails 内置)
github.com/miekg/dns                     # DNS 工具（Mihomo 已依赖）
go.uber.org/zap                          # 日志（替换 flexi_logger）
github.com/google/uuid                   # UUID 生成（替换 nanoid）
github.com/goreleaser/nfpm               # .deb/.rpm 打包工具
```

---

## 九、前端适配改动

原有 Tauri IPC：
```typescript
// 原来
import { invoke } from '@tauri-apps/api/core';
const config = await invoke<IVerge>('get_verge_config');
```

Wails 绑定替换：
```typescript
// 新的（自动生成 TypeScript 类型）
import { GetVergeConfig } from '../wailsjs/go/main/App';
const config = await GetVergeConfig();
```

**改动范围：** 仅需替换 `@tauri-apps/api` 调用层，业务逻辑组件不变。

---

## 十、构建与发布

```bash
# 开发模式
wails dev

# 生产构建
wails build -platform linux/amd64

# .deb 打包
nfpm package --packager deb --config nfpm.yaml

# 输出 (约 25-40MB)
dist/clashgo-linux-amd64.deb
```

```yaml
# nfpm.yaml
name: clashgo
arch: amd64
platform: linux
depends:
  - libwebkit2gtk-4.1-0
  - libayatana-appindicator3-1
  - openssl
```
