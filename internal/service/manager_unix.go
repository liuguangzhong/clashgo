//go:build !windows

package service

import "fmt"

// windowsIsAdmin Linux/macOS 上永远返回 false（无 Windows 管理员概念）
func (m *Manager) windowsIsAdmin() bool { return false }

func (m *Manager) installWindows() error {
	return fmt.Errorf("installWindows: not supported on this platform")
}

func (m *Manager) uninstallWindows() error { return nil }

func runElevatedPS(_ string) error {
	return fmt.Errorf("runElevatedPS: not supported on this platform")
}

func runElevated(_ string) error {
	return fmt.Errorf("runElevated: not supported on this platform")
}

func (m *Manager) queryWindows() ServiceStatus {
	return ServiceStatus{Available: false, Error: "not supported on this platform"}
}
