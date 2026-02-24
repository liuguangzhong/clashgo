// Package service Windows 系统服务管理
// 实现 TUN 模式所需的提权服务安装/卸载
// 对应原 src-tauri/src/core/service.rs
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"clashgo/internal/utils"
)

const serviceName = "ClashGoService"

// Manager 系统服务管理器
type Manager struct{}

// New 创建系统服务管理器
func New() *Manager {
	return &Manager{}
}

// IsAvailable 检查系统服务是否已安装并可用
// 对应原: is_service_available
func (m *Manager) IsAvailable() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("sc", "query", serviceName).Output()
	if err != nil {
		return false
	}
	return len(out) > 0
}

// Install 安装系统服务（需提权）
// 对应原: install_service
func (m *Manager) Install() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("system service is only supported on Windows")
	}
	svcPath, err := serviceExePath("install")
	if err != nil {
		return err
	}
	return runElevated(svcPath)
}

// Uninstall 卸载系统服务
// 对应原: uninstall_service
func (m *Manager) Uninstall() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("system service is only supported on Windows")
	}
	svcPath, err := serviceExePath("uninstall")
	if err != nil {
		return err
	}
	return runElevated(svcPath)
}

// Reinstall 重装系统服务（先卸载再安装）
// 对应原: reinstall_service
func (m *Manager) Reinstall() error {
	// 卸载时忽略错误（可能本来就没安装）
	_ = m.Uninstall()
	return m.Install()
}

// Repair 强制重新安装（对应 repair_service）
// 对应原: repair_service
func (m *Manager) Repair() error {
	return m.Reinstall()
}

// Start 启动已安装的系统服务
func (m *Manager) Start() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return exec.Command("sc", "start", serviceName).Run()
}

// Stop 停止系统服务
func (m *Manager) Stop() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return exec.Command("sc", "stop", serviceName).Run()
}

// ─── 内部帮助 ─────────────────────────────────────────────────────────────────

// serviceExePath 返回 install/uninstall 工具路径
// 这些工具与主程序同目录：clashgo-service-install.exe / clashgo-service-uninstall.exe
func serviceExePath(action string) (string, error) {
	exeDir, err := execDir()
	if err != nil {
		return "", fmt.Errorf("get exe dir: %w", err)
	}
	name := fmt.Sprintf("clashgo-service-%s.exe", action)
	path := filepath.Join(exeDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("service tool not found: %s", path)
	}
	return path, nil
}

// runElevated 在 Windows 上以管理员权限运行程序
// 通过 PowerShell Start-Process -Verb RunAs 实现提权
func runElevated(exePath string) error {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -Verb RunAs -Wait", exePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execDir 返回当前可执行文件所在目录
func execDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// UWPTool 调用 UWP 代理豁免工具（Windows 特有）
// 对应原: invoke_uwp_tool
func InvokeUWPTool() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("UWP tool is only available on Windows")
	}
	exeDir, err := execDir()
	if err != nil {
		return err
	}
	toolPath := filepath.Join(exeDir, "clashgo-uwp-tool.exe")
	if _, err := os.Stat(toolPath); err != nil {
		return fmt.Errorf("UWP tool not found: %s", toolPath)
	}
	return runElevated(toolPath)
}

// ServiceStatus 服务状态快照
type ServiceStatus struct {
	Available bool   `json:"available"`
	Running   bool   `json:"running"`
	Error     string `json:"error,omitempty"`
}

// QueryStatus 查询当前服务状态
func (m *Manager) QueryStatus() ServiceStatus {
	if runtime.GOOS != "windows" {
		return ServiceStatus{Available: false, Error: "not Windows"}
	}

	out, err := exec.Command("sc", "query", serviceName).Output()
	if err != nil {
		return ServiceStatus{Available: false, Error: err.Error()}
	}

	outStr := string(out)
	available := len(outStr) > 0

	_ = utils.Log() // ensure logger initialized
	running := false
	for i := 0; i < len(outStr)-7; i++ {
		if outStr[i:i+7] == "RUNNING" {
			running = true
			break
		}
	}

	return ServiceStatus{Available: available, Running: running}
}
