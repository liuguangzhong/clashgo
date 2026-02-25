package main

import (
	"context"
	"os"
	"os/exec"

	"clashgo/api"
	"clashgo/internal/config"
	"clashgo/internal/core"
	"clashgo/internal/hotkey"
	"clashgo/internal/notify"
	"clashgo/internal/proxy"
	"clashgo/internal/tray"
	"clashgo/internal/updater"
	"clashgo/internal/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// App 是整个应用的根结构体，通过 Wails 绑定到前端
type App struct {
	ctx context.Context

	// API 层（前端调用入口）
	configAPI      *api.ConfigAPI
	proxyAPI       *api.ProxyAPI
	profileAPI     *api.ProfileAPI
	systemAPI      *api.SystemAPI
	backupAPI      *api.BackupAPI
	serviceAPI     *api.ServiceAPI
	mediaUnlockAPI *api.MediaUnlockAPI

	// 内部模块
	coreManager *core.Manager
	sysProxy    proxy.SysProxy
	hotkeyMgr   *hotkey.Manager
	trayMgr     *tray.Manager
	appUpdater  *updater.Updater

	// 运行状态
	lightweight bool // 是否处于轻量模式
}

// NewApp 构造 App
// 注意: 此时必须创建 API 对象（即使是空壳），因为 Wails 在 Startup 之前
// 就会调用 Bindings() 进行绑定生成，要求所有值都是有效的结构体指针。
// 真正的依赖注入在 Startup() 中完成。
func NewApp() *App {
	return &App{
		configAPI:      &api.ConfigAPI{},
		proxyAPI:       &api.ProxyAPI{},
		profileAPI:     &api.ProfileAPI{},
		systemAPI:      &api.SystemAPI{},
		backupAPI:      &api.BackupAPI{},
		serviceAPI:     &api.ServiceAPI{},
		mediaUnlockAPI: &api.MediaUnlockAPI{},
	}
}

// Startup 由 Wails 在 WebView 就绪前调用，所有模块在此初始化
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	log := utils.Log()

	log.Info("Initializing ClashGo modules...")

	// 1. 初始化目录结构
	if err := utils.EnsureDirectories(); err != nil {
		log.Fatal("Failed to ensure directories", zap.Error(err))
	}

	// 2. 配置管理器
	cfgManager, err := config.NewManager()
	if err != nil {
		log.Fatal("Failed to initialize config manager", zap.Error(err))
	}

	// 3. API 层 — 通过 Init 注入依赖到已有对象（不替换指针，保持 Wails 绑定一致）
	a.configAPI.Init(cfgManager)
	a.proxyAPI.Init(cfgManager)
	a.profileAPI.Init(cfgManager)
	a.systemAPI.Init()
	a.backupAPI.Init(cfgManager)
	a.serviceAPI.Init()
	// mediaUnlockAPI 无需 Init（无内部状态）

	// 注入事件发送器（让 API 层可以向前端 emit 事件，无需持有 Wails ctx）
	api.SetEventEmitter(func(event string, data interface{}) {
		runtime.EventsEmit(ctx, event, data)
	})

	// 4. 代理核心
	a.coreManager = core.NewManager(cfgManager)
	if err := a.coreManager.Start(ctx); err != nil {
		log.Warn("Failed to start proxy core on startup", zap.Error(err))
	}

	// 5. 系统代理
	a.sysProxy = proxy.NewSysProxy()
	if err := a.sysProxy.Apply(cfgManager.GetVerge()); err != nil {
		log.Warn("Failed to apply system proxy settings", zap.Error(err))
	}

	// 6. 系统托盘
	a.trayMgr = tray.NewManager(ctx, a.coreManager, a.sysProxy, cfgManager)
	a.trayMgr.Start()

	// 7. 全局热键（注册动作回调）
	a.hotkeyMgr = hotkey.NewManager(cfgManager)
	a.hotkeyMgr.SetAction("toggle_system_proxy", func() {
		verge := cfgManager.GetVerge()
		enabled := verge.EnableSystemProxy == nil || *verge.EnableSystemProxy
		newVal := !enabled
		_ = cfgManager.PatchVerge(config.IVerge{EnableSystemProxy: &newVal})
		if newVal {
			_ = a.sysProxy.Apply(cfgManager.GetVerge())
		} else {
			_ = a.sysProxy.Reset()
		}
		runtime.EventsEmit(ctx, "verge:updated", nil)
	})
	a.hotkeyMgr.SetAction("open_dashboard", func() {
		runtime.WindowShow(ctx)
	})
	if err := a.hotkeyMgr.Register(); err != nil {
		log.Warn("Failed to register hotkeys", zap.Error(err))
	}

	// 8. 自动更新
	a.appUpdater = updater.NewUpdater(ctx, cfgManager)
	a.appUpdater.SetUpdateCallback(func(version, notes string) {
		runtime.EventsEmit(ctx, "app:update-available", map[string]string{
			"version": version,
			"notes":   notes,
		})
	})
	a.appUpdater.StartAutoCheck()

	// 9. Mihomo 日志流 → 前端事件
	a.coreManager.StartLogStream(func(level, msg string) {
		runtime.EventsEmit(ctx, "clash:log", map[string]string{
			"type":    level,
			"payload": msg,
		})
	})

	// 10. 注入 core.Manager 到 API 层
	api.SetCoreLifecycle(a.coreManager)

	// 11. 托盘代理选项同步：监听前端请求
	runtime.EventsOn(ctx, "tray:sync-proxy", func(_ ...interface{}) {
		a.trayMgr.UpdateMenu()
	})

	// 12. 轻量模式事件
	runtime.EventsOn(ctx, "lightweight:enter", func(_ ...interface{}) {
		a.lightweight = true
		runtime.WindowHide(ctx)
		log.Info("Entered lightweight mode")
	})
	runtime.EventsOn(ctx, "lightweight:exit", func(_ ...interface{}) {
		a.lightweight = false
		runtime.WindowShow(ctx)
		log.Info("Exited lightweight mode")
	})

	// 13. 单例激活回调（已有其他实例运行时显示窗口）
	utils.SetActivationCallback(func() {
		runtime.WindowShow(ctx)
	})

	log.Info("ClashGo initialization complete")
}

// DomReady 由 Wails 在前端 DOM 就绪后调用
func (a *App) DomReady(ctx context.Context) {
	verge := a.configAPI.GetVergeRaw()
	if verge.EnableSilentStart != nil && *verge.EnableSilentStart {
		return
	}
	runtime.WindowShow(ctx)
}

// Shutdown 由 Wails 在应用退出时调用
func (a *App) Shutdown(ctx context.Context) {
	a.gracefulShutdown()
}

// gracefulShutdown 有序关闭所有模块
func (a *App) gracefulShutdown() {
	log := utils.Log()
	log.Info("Graceful shutdown initiated...")

	if a.hotkeyMgr != nil {
		a.hotkeyMgr.Unregister()
	}
	if a.trayMgr != nil {
		a.trayMgr.Stop()
	}
	if a.sysProxy != nil {
		_ = a.sysProxy.Reset()
	}
	if a.coreManager != nil {
		_ = a.coreManager.Stop()
	}
	if a.appUpdater != nil {
		a.appUpdater.Stop()
	}

	log.Info("ClashGo shutdown complete")
}

// StartHidden 判断是否静默启动
func (a *App) StartHidden() bool {
	cfg, err := config.LoadVergeRaw()
	if err != nil {
		return false
	}
	if cfg.EnableSilentStart != nil {
		return *cfg.EnableSilentStart
	}
	return false
}

// Bindings 返回所有需要绑定到前端的对象
func (a *App) Bindings() []any {
	return []any{
		a,
		a.configAPI,
		a.proxyAPI,
		a.profileAPI,
		a.systemAPI,
		a.backupAPI,
		a.serviceAPI,
		a.mediaUnlockAPI,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 直接绑定到 App 的命令方法
// ─────────────────────────────────────────────────────────────────────────────

// NotifyUIReady 前端加载完成后调用
func (a *App) NotifyUIReady() {
	runtime.EventsEmit(a.ctx, "app:ready", nil)
	notify.Send("ClashGo", "已准备就绪")
}

// ExitApp 退出应用
func (a *App) ExitApp() {
	runtime.Quit(a.ctx)
}

// OpenDevtools 打开 WebView 开发者工具
// Wails v2 无内置 Go API 直接打开 devtools；
// 通过向前端发送 app:open-devtools 事件，由前端 JS 调用浏览器内置 API
// 前端收到事件后可执行: window.open('chrome://inspect') 或调用内部注册的 devtools 钩子
func (a *App) OpenDevtools() {
	// 方案1: 通知前端（前端需监听 app:open-devtools 并自行处理）
	runtime.EventsEmit(a.ctx, "app:open-devtools", nil)

	// 方案2: 直接在 WebView 内执行 JS（兼容前端已注册 __wails_devtools_open 的情况）
	runtime.WindowExecJS(a.ctx, `
		if (typeof window.__wails_devtools_open === 'function') {
			window.__wails_devtools_open();
		} else if (typeof window.__wails !== 'undefined' && window.__wails.openBrowserDevTools) {
			window.__wails.openBrowserDevTools();
		}
	`)
}

// GetRunningMode 获取核心当前运行模式
func (a *App) GetRunningMode() string {
	if a.coreManager == nil {
		return "NotRunning"
	}
	return a.coreManager.RunningMode()
}

// SyncTrayProxySelection 直接触发托盘代理选项刷新
// 对应原: sync_tray_proxy_selection
// 前端调用此方法后托盘菜单立即更新，无需二次 EventsEmit
func (a *App) SyncTrayProxySelection() {
	if a.trayMgr != nil {
		a.trayMgr.UpdateMenu()
	}
}

// IsLightweightMode 返回当前是否处于轻量模式
func (a *App) IsLightweightMode() bool {
	return a.lightweight
}

// EntryLightweightMode 进入轻量后台模式（隐藏窗口，减少资源占用）
// 对应原: entry_lightweight_mode
func (a *App) EntryLightweightMode() {
	a.lightweight = true
	runtime.WindowHide(a.ctx)
	utils.Log().Info("Entered lightweight mode")
}

// ExitLightweightMode 退出轻量模式，显示主窗口
// 对应原: exit_lightweight_mode
func (a *App) ExitLightweightMode() {
	a.lightweight = false
	runtime.WindowShow(a.ctx)
	utils.Log().Info("Exited lightweight mode")
}

// RestartApp 重启整个应用（重新执行同一二进制）
func (a *App) RestartApp() {
	exePath, err := os.Executable()
	if err != nil {
		utils.Log().Error("RestartApp: cannot get executable path", zap.Error(err))
		return
	}
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		utils.Log().Error("RestartApp: failed to start new process", zap.Error(err))
		return
	}
	runtime.Quit(a.ctx)
}
