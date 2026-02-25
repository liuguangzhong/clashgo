//go:build windows

package proxy

import (
	"clashgo/internal/config"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// windowsProxy Windows 系统代理（通过注册表 WinINet）
type windowsProxy struct{}

func newPlatformProxy() SysProxy { return &windowsProxy{} }

const (
	inetRegPath = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

func (p *windowsProxy) Apply(verge config.IVerge) error {
	sysEnabled := verge.EnableSystemProxy != nil && *verge.EnableSystemProxy

	if !sysEnabled {
		return p.Reset()
	}

	host := "127.0.0.1"
	if verge.ProxyHost != nil {
		host = *verge.ProxyHost
	}
	port := 17897
	if verge.VergeMixedPort != nil {
		port = int(*verge.VergeMixedPort)
	}
	bypass := getBypass(verge)

	k, err := registry.OpenKey(registry.CURRENT_USER, inetRegPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry key: %w", err)
	}
	defer k.Close()

	proxyServer := host + ":" + strconv.Itoa(port)

	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyServer", proxyServer); err != nil {
		return err
	}
	if err := k.SetStringValue("ProxyOverride", bypass); err != nil {
		return err
	}

	// 通知系统设置已更改
	return refreshInetSettings()
}

func (p *windowsProxy) Reset() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, inetRegPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	_ = k.SetDWordValue("ProxyEnable", 0)
	return refreshInetSettings()
}

func (p *windowsProxy) GetCurrentProxy() (*ProxyInfo, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, inetRegPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()

	enabled, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	bypass, _, _ := k.GetStringValue("ProxyOverride")

	info := &ProxyInfo{
		Enabled: enabled == 1,
		Bypass:  bypass,
	}

	if server != "" {
		// 解析 "host:port"
		fmt.Sscanf(server, "%99[^:]:%d", &info.Host, &info.Port)
	}

	return info, nil
}

// hiddenCmd 创建一个隐藏控制台窗口的 exec.Cmd
func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

// refreshInetSettings 通知所有应用系统代理设置已更改
// 使用 InternetSetOption Win32 API，不需要管理员权限
func refreshInetSettings() error {
	wininet, err := syscall.LoadDLL("wininet.dll")
	if err != nil {
		// fallback: 忽略错误，注册表已设置好
		return nil
	}
	defer wininet.Release()

	proc, err := wininet.FindProc("InternetSetOptionW")
	if err != nil {
		return nil
	}

	// INTERNET_OPTION_SETTINGS_CHANGED = 39
	// INTERNET_OPTION_REFRESH = 37
	proc.Call(0, 39, 0, 0)
	proc.Call(0, 37, 0, 0)

	_ = unsafe.Pointer(nil) // keep import
	return nil
}
