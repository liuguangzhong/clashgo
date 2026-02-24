//go:build darwin

package proxy

import (
	"clashgo/internal/config"
	"os/exec"
	"strconv"
)

// macProxy macOS proxy via networksetup
type macProxy struct{}

func newPlatformProxy() SysProxy { return &macProxy{} }

func (p *macProxy) Apply(verge config.IVerge) error {
	host := "127.0.0.1"
	if verge.ProxyHost != nil {
		host = *verge.ProxyHost
	}
	port := 7897
	if verge.VergeMixedPort != nil {
		port = int(*verge.VergeMixedPort)
	}
	portStr := strconv.Itoa(port)
	svc := "Wi-Fi"

	if verge.EnableSystemProxy != nil && *verge.EnableSystemProxy {
		_ = exec.Command("networksetup", "-setwebproxy", svc, host, portStr).Run()
		_ = exec.Command("networksetup", "-setsecurewebproxy", svc, host, portStr).Run()
		_ = exec.Command("networksetup", "-setsocksfirewallproxy", svc, host, portStr).Run()
	} else {
		return p.Reset()
	}
	return nil
}

func (p *macProxy) Reset() error {
	svc := "Wi-Fi"
	_ = exec.Command("networksetup", "-setwebproxystate", svc, "off").Run()
	_ = exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run()
	_ = exec.Command("networksetup", "-setsocksfirewallproxystate", svc, "off").Run()
	return nil
}

func (p *macProxy) GetCurrentProxy() (*ProxyInfo, error) {
	return &ProxyInfo{}, nil
}
