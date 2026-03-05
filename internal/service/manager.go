// Package service 系统服务管理（跨平台）
// 实现 TUN 模式所需的提权服务安装/卸载
// Windows: sc 安装 Windows Service
// Linux: setcap 或 systemd service
// 对应原 src-tauri/src/core/service.rs
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"clashgo/internal/utils"

	"go.uber.org/zap"
)

const serviceName = "ClashGoService"

// Manager 系统服务管理器
type Manager struct{}

// New 创建系统服务管理器
func New() *Manager {
	return &Manager{}
}

// IsAvailable 检查 TUN 模式是否可用
func (m *Manager) IsAvailable() bool {
	switch runtime.GOOS {
	case "windows":
		// Windows TUN 需要管理员权限运行
		return m.windowsIsAdmin()
	case "linux":
		// Linux: 检查二进制是否有 cap_net_admin 能力
		return m.linuxHasCapability()
	default:
		return false
	}
}

// Install 安装系统服务（需提权）
func (m *Manager) Install() error {
	switch runtime.GOOS {
	case "windows":
		return m.installWindows()
	case "linux":
		return m.installLinux()
	default:
		return fmt.Errorf("system service not supported on %s", runtime.GOOS)
	}
}

// Uninstall 卸载系统服务
func (m *Manager) Uninstall() error {
	switch runtime.GOOS {
	case "windows":
		return m.uninstallWindows()
	case "linux":
		return m.uninstallLinux()
	default:
		return fmt.Errorf("system service not supported on %s", runtime.GOOS)
	}
}

// Reinstall 重装系统服务
func (m *Manager) Reinstall() error {
	_ = m.Uninstall()
	return m.Install()
}

// Repair 强制重新安装
func (m *Manager) Repair() error {
	return m.Reinstall()
}

// Start 启动已安装的系统服务
func (m *Manager) Start() error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("sc", "start", serviceName).Run()
	case "linux":
		// Linux 下 setcap 模式不需要 start/stop 服务
		// 如果用 systemd service 则：
		return exec.Command("systemctl", "start", "clashgo").Run()
	}
	return nil
}

// Stop 停止系统服务
func (m *Manager) Stop() error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("sc", "stop", serviceName).Run()
	case "linux":
		return exec.Command("systemctl", "stop", "clashgo").Run()
	}
	return nil
}

// ─── Windows 实现（见 manager_windows.go）────────────────────────────────────

// ─── Linux 实现 ──────────────────────────────────────────────────────────────

// installLinux 通过 pkexec（图形化提权）给 ClashGo 二进制设置 cap_net_admin
// 这样 TUN 模式可以在非 root 用户下创建虚拟网卡
func (m *Manager) installLinux() error {
	log := utils.Log()

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// 使用 pkexec（Polkit 图形提权）或 sudo
	setcapCmd := fmt.Sprintf("setcap cap_net_bind_service,cap_net_admin=+ep '%s'", exePath)

	// 优先尝试 pkexec（图形界面弹窗提权）
	if _, err := exec.LookPath("pkexec"); err == nil {
		log.Info("Installing TUN capability via pkexec", zap.String("exe", exePath))
		cmd := exec.Command("pkexec", "bash", "-c", setcapCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Warn("pkexec setcap failed", zap.String("output", string(out)), zap.Error(err))
			return fmt.Errorf("pkexec setcap failed: %s (output: %s)", err, string(out))
		}
		log.Info("TUN capability set successfully via pkexec")
		return nil
	}

	// fallback: 尝试 sudo（终端模式）
	if _, err := exec.LookPath("sudo"); err == nil {
		log.Info("Installing TUN capability via sudo", zap.String("exe", exePath))
		cmd := exec.Command("sudo", "bash", "-c", setcapCmd)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("sudo setcap failed: %w", err)
		}
		log.Info("TUN capability set successfully via sudo")
		return nil
	}

	return fmt.Errorf("cannot set TUN capability: neither pkexec nor sudo available. Run manually:\nsudo %s", setcapCmd)
}

// uninstallLinux 移除二进制的 capability
func (m *Manager) uninstallLinux() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	removeCmd := fmt.Sprintf("setcap -r '%s'", exePath)

	if _, err := exec.LookPath("pkexec"); err == nil {
		cmd := exec.Command("pkexec", "bash", "-c", removeCmd)
		return cmd.Run()
	}
	if _, err := exec.LookPath("sudo"); err == nil {
		cmd := exec.Command("sudo", "bash", "-c", removeCmd)
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}
	return fmt.Errorf("cannot remove capability: run manually:\nsudo %s", removeCmd)
}

// linuxHasCapability 检查当前二进制是否有 cap_net_admin 能力
func (m *Manager) linuxHasCapability() bool {
	exePath, err := os.Executable()
	if err != nil {
		return false
	}

	// 方式1: 如果以 root 运行，直接有权限
	if os.Geteuid() == 0 {
		return true
	}

	// 方式2: 检查 getcap 输出
	out, err := exec.Command("getcap", exePath).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "cap_net_admin")
}

// ─── 通用 ────────────────────────────────────────────────────────────────────

// execDir 返回当前可执行文件所在目录
func execDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// UWPTool 调用 UWP 代理豁免工具（Windows 特有）
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
	switch runtime.GOOS {
	case "windows":
		return m.queryWindows()
	case "linux":
		return ServiceStatus{
			Available: m.linuxHasCapability(),
			Running:   m.linuxHasCapability(), // 有 cap 即可用
		}
	default:
		return ServiceStatus{Available: false, Error: "unsupported OS"}
	}
}
