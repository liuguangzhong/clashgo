// proxy/sysproxy.go - 系统代理跨平台接口定义
// 对应原 src-tauri/src/core/sysopt.rs

package proxy

import (
	"runtime"

	"clashgo/internal/config"
)

// SysProxy 系统代理操作接口（跨平台）
type SysProxy interface {
	// Apply 根据 verge 配置设置系统代理
	Apply(verge config.IVerge) error
	// Reset 关闭所有系统代理设置
	Reset() error
	// GetCurrentProxy 获取当前系统代理信息
	GetCurrentProxy() (*ProxyInfo, error)
}

// ProxyInfo 系统代理信息
type ProxyInfo struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Bypass  string `json:"bypass"`
}

// NewSysProxy 根据当前平台返回对应的系统代理实现
// 各平台实现在对应的 build-tag 文件中（linux.go / windows.go / darwin.go）
func NewSysProxy() SysProxy {
	return newPlatformProxy()
}

// ─── Linux 专属 bypass 默认值 ────────────────────────────────────────────────

const defaultBypassLinux = "localhost,127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,::1"
const defaultBypassWindows = "localhost;127.*;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;<local>"
const defaultBypassMac = "127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,localhost,*.local,*.crashlytics.com,<local>"

// getBypass 计算最终 bypass 字符串（对应原 get_bypass()）
func getBypass(verge config.IVerge) string {
	useDefault := verge.UseDefaultBypass == nil || *verge.UseDefaultBypass

	var defaultBypass string
	switch runtime.GOOS {
	case "linux":
		defaultBypass = defaultBypassLinux
	case "windows":
		defaultBypass = defaultBypassWindows
	case "darwin":
		defaultBypass = defaultBypassMac
	default:
		defaultBypass = "localhost,127.0.0.1"
	}

	custom := ""
	if verge.SystemProxyBypass != nil {
		custom = *verge.SystemProxyBypass
	}

	if custom == "" {
		return defaultBypass
	}
	if useDefault {
		return defaultBypass + "," + custom
	}
	return custom
}

// noopProxy 不支持的平台使用 noop 实现
type noopProxy struct{}

func (n *noopProxy) Apply(_ config.IVerge) error            { return nil }
func (n *noopProxy) Reset() error                           { return nil }
func (n *noopProxy) GetCurrentProxy() (*ProxyInfo, error)   { return &ProxyInfo{}, nil }
