package tray

import (
	"context"
	"fmt"

	"clashgo/internal/config"
	"clashgo/internal/core"
	"clashgo/internal/proxy"
	"clashgo/internal/utils"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"
)

// Manager 系统托盘管理器（对应原 src-tauri/src/core/tray/mod.rs）
type Manager struct {
	ctx    context.Context
	core   *core.Manager
	proxy  proxy.SysProxy
	cfgMgr *config.Manager
	appMenu *menu.Menu
}

// NewManager 创建托盘管理器
func NewManager(ctx context.Context, cm *core.Manager, sp proxy.SysProxy, cfg *config.Manager) *Manager {
	return &Manager{ctx: ctx, core: cm, proxy: sp, cfgMgr: cfg}
}

// Start 初始化系统托盘及应用菜单
func (m *Manager) Start() {
	m.appMenu = m.buildMenu()
	runtime.MenuSetApplicationMenu(m.ctx, m.appMenu)
	runtime.MenuUpdateApplicationMenu(m.ctx)
	utils.Log().Info("System tray initialized")
}

// Stop 销毁系统托盘
func (m *Manager) Stop() {
	utils.Log().Info("System tray stopped")
}

// UpdateMenu 动态重建并刷新菜单（代理状态变化时调用）
func (m *Manager) UpdateMenu() {
	m.appMenu = m.buildMenu()
	runtime.MenuSetApplicationMenu(m.ctx, m.appMenu)
	runtime.MenuUpdateApplicationMenu(m.ctx)
}

// buildMenu 根据当前状态构建完整菜单
// 对应原 tray/mod.rs build_system_tray_menu()
func (m *Manager) buildMenu() *menu.Menu {
	verge := m.cfgMgr.GetVerge()

	sysProxyEnabled := verge.EnableSystemProxy != nil && *verge.EnableSystemProxy
	tunEnabled := verge.EnableTunMode != nil && *verge.EnableTunMode
	isRunning := m.core != nil && m.core.IsRunning()

	appMenu := menu.NewMenu()

	// ── 状态区 ────────────────────────────────────────────────────────────────
	statusLabel := "● 运行中"
	if !isRunning {
		statusLabel = "○ 已停止"
	}
	appMenu.Append(menu.Label(statusLabel))
	appMenu.Append(menu.Separator())

	// ── 窗口控制 ──────────────────────────────────────────────────────────────
	appMenu.Append(menu.SubMenu("ClashGo", menu.NewMenuFromItems(
		menu.Text("显示主界面", keys.CmdOrCtrl("w"), func(_ *menu.CallbackData) {
			runtime.WindowShow(m.ctx)
		}),
		menu.Text("隐藏主界面", nil, func(_ *menu.CallbackData) {
			runtime.WindowHide(m.ctx)
		}),
		menu.Separator(),
		menu.Text("退出 ClashGo", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
			m.handleQuit()
		}),
	)))

	appMenu.Append(menu.Separator())

	// ── 代理模式 ──────────────────────────────────────────────────────────────
	appMenu.Append(menu.SubMenu("代理模式", m.buildModeMenu()))

	// ── 系统代理开关 ──────────────────────────────────────────────────────────
	sysProxyLabel := "启用系统代理"
	if sysProxyEnabled {
		sysProxyLabel = "✓ 系统代理"
	}
	appMenu.Append(menu.Text(sysProxyLabel, keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) {
		m.toggleSystemProxy(!sysProxyEnabled)
	}))

	// ── TUN 模式开关 ──────────────────────────────────────────────────────────
	tunLabel := "启用 TUN 模式"
	if tunEnabled {
		tunLabel = "✓ TUN 模式"
	}
	appMenu.Append(menu.Text(tunLabel, keys.CmdOrCtrl("t"), func(_ *menu.CallbackData) {
		m.toggleTun(!tunEnabled)
	}))

	appMenu.Append(menu.Separator())

	// ── 核心控制 ──────────────────────────────────────────────────────────────
	appMenu.Append(menu.SubMenu("代理核心", menu.NewMenuFromItems(
		menu.Text("重载配置", nil, func(_ *menu.CallbackData) {
			m.reloadConfig()
		}),
		menu.Text(func() string {
			if isRunning {
				return "停止内核"
			}
			return "启动内核"
		}(), nil, func(_ *menu.CallbackData) {
			m.toggleCore(isRunning)
		}),
		menu.Text("重启内核", nil, func(_ *menu.CallbackData) {
			m.restartCore()
		}),
	)))

	return appMenu
}

// ── 代理模式子菜单 ────────────────────────────────────────────────────────────

func (m *Manager) buildModeMenu() *menu.Menu {
	modes := []struct {
		Label string
		Value string
	}{
		{"规则模式 (Rule)", "rule"},
		{"全局模式 (Global)", "global"},
		{"直连模式 (Direct)", "direct"},
	}

	clash := m.cfgMgr.GetClash()
	currentMode, _ := clash["mode"].(string)

	modeMenu := menu.NewMenu()
	for _, mode := range modes {
		modeVal := mode.Value // capture loop var
		label := mode.Label
		if currentMode == modeVal {
			label = "✓ " + label
		}
		modeMenu.Append(menu.Text(label, nil, func(_ *menu.CallbackData) {
			m.switchMode(modeVal)
		}))
	}
	return modeMenu
}

// ── 操作实现 ─────────────────────────────────────────────────────────────────

func (m *Manager) toggleSystemProxy(enable bool) {
	val := enable
	if err := m.cfgMgr.PatchVerge(config.IVerge{EnableSystemProxy: &val}); err != nil {
		utils.Log().Error("Failed to patch system proxy config", zap.Error(err))
		return
	}

	verge := m.cfgMgr.GetVerge()
	if enable {
		if err := m.proxy.Apply(verge); err != nil {
			utils.Log().Error("Failed to enable system proxy", zap.Error(err))
		}
	} else {
		if err := m.proxy.Reset(); err != nil {
			utils.Log().Error("Failed to disable system proxy", zap.Error(err))
		}
	}

	// 通知前端刷新 UI
	runtime.EventsEmit(m.ctx, "verge:updated", nil)
	m.UpdateMenu()
}

func (m *Manager) toggleTun(enable bool) {
	val := enable
	if err := m.cfgMgr.PatchVerge(config.IVerge{EnableTunMode: &val}); err != nil {
		utils.Log().Error("Failed to patch TUN config", zap.Error(err))
		return
	}
	if err := m.core.UpdateConfig(); err != nil {
		utils.Log().Error("Failed to reload config for TUN", zap.Error(err))
	}
	runtime.EventsEmit(m.ctx, "verge:updated", nil)
	m.UpdateMenu()
}

func (m *Manager) switchMode(mode string) {
	if err := m.cfgMgr.PatchClash(map[string]interface{}{"mode": mode}); err != nil {
		utils.Log().Error("Failed to switch mode", zap.Error(err))
		return
	}
	if err := m.core.UpdateConfig(); err != nil {
		utils.Log().Error("Failed to reload after mode switch", zap.Error(err))
	}
	runtime.EventsEmit(m.ctx, "clash:updated", nil)
	m.UpdateMenu()
}

func (m *Manager) reloadConfig() {
	if err := m.core.UpdateConfig(); err != nil {
		utils.Log().Error("Config reload failed from tray", zap.Error(err))
		runtime.MessageDialog(m.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "重载失败",
			Message: fmt.Sprintf("配置重载失败: %v", err),
		})
		return
	}
	utils.Log().Info("Config reloaded from tray")
}

func (m *Manager) toggleCore(isRunning bool) {
	if isRunning {
		if err := m.core.Stop(); err != nil {
			utils.Log().Error("Stop core failed", zap.Error(err))
		}
	} else {
		if err := m.core.Start(m.ctx); err != nil {
			utils.Log().Error("Start core failed", zap.Error(err))
		}
	}
	runtime.EventsEmit(m.ctx, "core:updated", nil)
	m.UpdateMenu()
}

func (m *Manager) restartCore() {
	if err := m.core.Restart(); err != nil {
		utils.Log().Error("Restart core failed", zap.Error(err))
		runtime.MessageDialog(m.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "重启失败",
			Message: fmt.Sprintf("内核重启失败: %v", err),
		})
	}
	runtime.EventsEmit(m.ctx, "core:updated", nil)
	m.UpdateMenu()
}

func (m *Manager) handleQuit() {
	runtime.Quit(m.ctx)
}
