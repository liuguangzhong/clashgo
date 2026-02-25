// Package hotkey 全局热键管理
// 使用 golang.design/x/hotkey（跨平台：Windows/Linux X11/macOS）
// 对应原 src-tauri/src/core/hotkey.rs
package hotkey

import (
	"fmt"
	"strings"
	"sync"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	hotkey "golang.design/x/hotkey"
	"go.uber.org/zap"
)

// Manager 全局热键管理器
type Manager struct {
	cfgMgr    *config.Manager
	mu        sync.Mutex
	registered []*hotkey.Hotkey
	actions   map[string]func() // 热键名 → 执行函数
}

// NewManager 创建热键管理器
func NewManager(cfg *config.Manager) *Manager {
	return &Manager{
		cfgMgr:  cfg,
		actions: make(map[string]func()),
	}
}

// SetAction 注册热键动作（由 app.go 注入）
// name: "toggle_system_proxy" / "open_dashboard" 等
func (m *Manager) SetAction(name string, fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions[name] = fn
}

// Register 注册所有在 verge.yaml 中配置的全局热键
// 热键格式: "toggle_system_proxy|Ctrl+Shift+P"
func (m *Manager) Register() error {
	verge := m.cfgMgr.GetVerge()
	if verge.EnableGlobalHotkey == nil || !*verge.EnableGlobalHotkey {
		return nil
	}
	if verge.Hotkeys == nil || len(*verge.Hotkeys) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	log := utils.Log()
	var errs []string

	for _, entry := range *verge.Hotkeys {
		// 格式: "action_name|Ctrl+Shift+P"
		parts := strings.SplitN(entry, "|", 2)
		if len(parts) != 2 {
			log.Warn("Invalid hotkey format", zap.String("entry", entry))
			continue
		}
		actionName := strings.TrimSpace(parts[0])
		keyCombo := strings.TrimSpace(parts[1])

		mods, key, err := parseKeyCombo(keyCombo)
		if err != nil {
			log.Warn("Cannot parse hotkey", zap.String("combo", keyCombo), zap.Error(err))
			errs = append(errs, err.Error())
			continue
		}

		hk := hotkey.New(mods, key)
		if err := hk.Register(); err != nil {
			log.Warn("Failed to register hotkey",
				zap.String("combo", keyCombo),
				zap.Error(err))
			errs = append(errs, fmt.Sprintf("%s: %v", keyCombo, err))
			continue
		}

		m.registered = append(m.registered, hk)
		name := actionName // capture

		// 在独立 goroutine 中监听热键触发
		go func() {
			for range hk.Keydown() {
				m.mu.Lock()
				fn := m.actions[name]
				m.mu.Unlock()
				if fn != nil {
					fn()
				} else {
					log.Debug("Hotkey triggered but no action registered",
						zap.String("action", name))
				}
			}
		}()

		log.Info("Hotkey registered",
			zap.String("action", actionName),
			zap.String("combo", keyCombo))
	}

	if len(errs) > 0 {
		return fmt.Errorf("some hotkeys failed to register: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Unregister 注销所有已注册的全局热键
func (m *Manager) Unregister() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, hk := range m.registered {
		_ = hk.Unregister()
	}
	m.registered = nil
	utils.Log().Info("Global hotkeys unregistered")
}

// parseKeyCombo 将 "Ctrl+Shift+P" 解析为 golang.design/x/hotkey 的 mods + key
func parseKeyCombo(combo string) ([]hotkey.Modifier, hotkey.Key, error) {
	parts := strings.Split(combo, "+")
	if len(parts) == 0 {
		return nil, 0, fmt.Errorf("empty key combo")
	}

	var mods []hotkey.Modifier
	// 最后一个部分是主键，前面的都是修饰键
	for _, part := range parts[:len(parts)-1] {
		mod, err := parseMod(strings.TrimSpace(part))
		if err != nil {
			return nil, 0, err
		}
		mods = append(mods, mod)
	}

	key, err := parseKey(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil {
		return nil, 0, err
	}

	return mods, key, nil
}

func parseMod(s string) (hotkey.Modifier, error) {
	switch strings.ToLower(s) {
	case "ctrl", "control":
		return hotkey.ModCtrl, nil
	case "shift":
		return hotkey.ModShift, nil
	case "alt", "option":
		return modAlt, nil
	case "super", "win", "cmd", "command":
		return modWin, nil
	}
	return 0, fmt.Errorf("unknown modifier: %s", s)
}

func parseKey(s string) (hotkey.Key, error) {
	// 字母键
	if len(s) == 1 {
		c := strings.ToUpper(s)[0]
		if c >= 'A' && c <= 'Z' {
			return hotkey.Key(hotkey.KeyA + hotkey.Key(c-'A')), nil
		}
		if c >= '0' && c <= '9' {
			return hotkey.Key(hotkey.Key0 + hotkey.Key(c-'0')), nil
		}
	}

	// 特殊键
	switch strings.ToLower(s) {
	case "space":
		return hotkey.KeySpace, nil
	case "enter", "return":
		return hotkey.KeyReturn, nil
	case "escape", "esc":
		return hotkey.KeyEscape, nil
	case "tab":
		return hotkey.KeyTab, nil
	case "f1":
		return hotkey.KeyF1, nil
	case "f2":
		return hotkey.KeyF2, nil
	case "f3":
		return hotkey.KeyF3, nil
	case "f4":
		return hotkey.KeyF4, nil
	case "f5":
		return hotkey.KeyF5, nil
	case "f6":
		return hotkey.KeyF6, nil
	case "f7":
		return hotkey.KeyF7, nil
	case "f8":
		return hotkey.KeyF8, nil
	case "f9":
		return hotkey.KeyF9, nil
	case "f10":
		return hotkey.KeyF10, nil
	case "f11":
		return hotkey.KeyF11, nil
	case "f12":
		return hotkey.KeyF12, nil
	}
	return 0, fmt.Errorf("unknown key: %s", s)
}
