// Package utils - 自启动管理（对应原 src-tauri/src/core/sysopt.rs autolaunch）
// 跨平台：Linux ~/.config/autostart/.desktop / Windows 注册表 / macOS LaunchAgent

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
)

// AutoStartManager 自启动管理器
type AutoStartManager struct{}

// NewAutoStart 创建自启动管理器实例
func NewAutoStart() *AutoStartManager {
	return &AutoStartManager{}
}

// Enable 启用开机自启动
func (a *AutoStartManager) Enable() error {
	switch runtime.GOOS {
	case "linux":
		return a.enableLinux()
	case "windows":
		return a.enableWindows()
	case "darwin":
		return a.enableMacOS()
	default:
		return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
	}
}

// Disable 禁用开机自启动
func (a *AutoStartManager) Disable() error {
	switch runtime.GOOS {
	case "linux":
		return a.disableLinux()
	case "windows":
		return a.disableWindows()
	case "darwin":
		return a.disableMacOS()
	default:
		return nil
	}
}

// IsEnabled 检查是否已启用自启动
func (a *AutoStartManager) IsEnabled() bool {
	switch runtime.GOOS {
	case "linux":
		_, err := os.Stat(a.linuxDesktopPath())
		return err == nil
	case "windows":
		return a.isEnabledWindows()
	case "darwin":
		_, err := os.Stat(a.macLaunchAgentPath())
		return err == nil
	}
	return false
}

// ─── Linux 实现（XDG Autostart）─────────────────────────────────────────────

const linuxDesktopTemplate = `[Desktop Entry]
Type=Application
Name=ClashGo
Exec={{.ExecPath}} --start-hidden
Icon=clashgo
StartupNotify=false
Comment=Clash Verge Go Edition
Categories=Network;Proxy;
X-GNOME-Autostart-enabled=true
`

func (a *AutoStartManager) enableLinux() error {
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		home, _ := os.UserHomeDir()
		xdgConfig = filepath.Join(home, ".config")
	}
	autostartDir := filepath.Join(xdgConfig, "autostart")
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("create autostart dir: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	tmpl, _ := template.New("desktop").Parse(linuxDesktopTemplate)
	f, err := os.Create(a.linuxDesktopPath())
	if err != nil {
		return fmt.Errorf("create desktop file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, map[string]string{"ExecPath": exePath})
}

func (a *AutoStartManager) disableLinux() error {
	path := a.linuxDesktopPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

func (a *AutoStartManager) linuxDesktopPath() string {
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" {
		home, _ := os.UserHomeDir()
		xdgConfig = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfig, "autostart", "clashgo.desktop")
}

// ─── Windows 实现（注册表 Run 键）────────────────────────────────────────────

func (a *AutoStartManager) enableWindows() error {
	// 通过 reg add HKCU\Software\Microsoft\Windows\CurrentVersion\Run
	return regSetAutostart(true)
}

func (a *AutoStartManager) disableWindows() error {
	return regSetAutostart(false)
}

func (a *AutoStartManager) isEnabledWindows() bool {
	return regIsAutostartEnabled()
}

// ─── macOS 实现（LaunchAgent plist）─────────────────────────────────────────

const macPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.clashgo.app</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExecPath}}</string>
        <string>--start-hidden</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`

func (a *AutoStartManager) enableMacOS() error {
	dir := filepath.Dir(a.macLaunchAgentPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	exePath, _ := os.Executable()
	tmpl, _ := template.New("plist").Parse(macPlistTemplate)
	f, err := os.Create(a.macLaunchAgentPath())
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, map[string]string{"ExecPath": exePath})
}

func (a *AutoStartManager) disableMacOS() error {
	path := a.macLaunchAgentPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(path)
}

func (a *AutoStartManager) macLaunchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "dev.clashgo.app.plist")
}
