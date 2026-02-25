//go:build windows

package proxy

import (
	"clashgo/internal/config"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"

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
	port := 7897
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
func refreshInetSettings() error {
	return hiddenCmd("netsh", "winhttp", "import", "proxy", "source=ie").Run()
}
