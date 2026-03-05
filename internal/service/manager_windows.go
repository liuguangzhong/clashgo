//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// windowsIsAdmin 检查当前进程是否以管理员身份运行
func (m *Manager) windowsIsAdmin() bool {
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	return err == nil
}

// installWindows Windows 上以管理员身份重启当前程序以获取 TUN 权限
func (m *Manager) installWindows() error {
	if m.windowsIsAdmin() {
		return nil // 已经是管理员
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// 通过 ShellExecute 以管理员身份重启自身
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs", exePath))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart as admin: %w", err)
	}

	// 新进程已启动，当前进程应该退出
	os.Exit(0)
	return nil
}

func (m *Manager) uninstallWindows() error {
	// Windows 不需要卸载，只需普通用户重启即可
	return nil
}

// runElevatedPS 通过 PowerShell 以管理员权限执行脚本（隐藏窗口）
func runElevatedPS(script string) error {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process powershell -Verb RunAs -WindowStyle Hidden -Wait -ArgumentList '-Command','%s'", script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// runElevated 在 Windows 上以管理员权限运行（隐藏窗口）
func runElevated(exePath string) error {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs -Wait -WindowStyle Hidden", exePath))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

func (m *Manager) queryWindows() ServiceStatus {
	isAdmin := m.windowsIsAdmin()
	return ServiceStatus{
		Available: isAdmin,
		Running:   isAdmin,
	}
}
