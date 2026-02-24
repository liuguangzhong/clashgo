//go:build linux

package proxy

import (
	"fmt"
	"os/exec"
	"strconv"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

// linuxProxy Linux 系统代理实现
// 优先使用 D-Bus 调用 GNOME gsettings，fallback 到 KDE kwriteconfig / 环境变量
type linuxProxy struct {
	conn        *dbus.Conn
	desktopType string // "gnome" | "kde" | "env"
}

func newLinuxProxy() SysProxy {
	p := &linuxProxy{}
	p.detectDesktop()
	return p
}

// detectDesktop 检测当前桌面环境
func (p *linuxProxy) detectDesktop() {
	// 检查 GNOME
	conn, err := dbus.ConnectSessionBus()
	if err == nil {
		obj := conn.Object("org.gnome.system.proxy", "/org/gnome/system/proxy")
		call := obj.Call("org.freedesktop.DBus.Peer.Ping", 0)
		if call.Err == nil {
			p.conn = conn
			p.desktopType = "gnome"
			utils.Log().Info("Detected GNOME desktop, using D-Bus for system proxy")
			return
		}
		conn.Close()
	}

	// 检查 KDE
	if _, err := exec.LookPath("kwriteconfig5"); err == nil {
		p.desktopType = "kde"
		utils.Log().Info("Detected KDE desktop, using kwriteconfig5 for system proxy")
		return
	}
	if _, err := exec.LookPath("kwriteconfig6"); err == nil {
		p.desktopType = "kde"
		utils.Log().Info("Detected KDE Plasma 6, using kwriteconfig6 for system proxy")
		return
	}

	// fallback: 环境变量（仅当前进程生效，实际意义有限）
	p.desktopType = "env"
	utils.Log().Warn("Unknown desktop environment, system proxy may not work correctly")
}

// Apply 设置系统代理
func (p *linuxProxy) Apply(verge config.IVerge) error {
	sysEnabled := verge.EnableSystemProxy != nil && *verge.EnableSystemProxy
	pacEnabled := verge.ProxyAutoConfig != nil && *verge.ProxyAutoConfig

	host := "127.0.0.1"
	if verge.ProxyHost != nil {
		host = *verge.ProxyHost
	}

	port := 7897
	if verge.VergeMixedPort != nil {
		port = int(*verge.VergeMixedPort)
	}

	bypass := getBypass(verge)

	if !sysEnabled && !pacEnabled {
		return p.Reset()
	}

	switch p.desktopType {
	case "gnome":
		return p.applyGNOME(host, port, bypass, pacEnabled, verge)
	case "kde":
		return p.applyKDE(host, port, bypass, pacEnabled)
	default:
		utils.Log().Warn("Cannot set system proxy: unsupported desktop environment")
		return nil
	}
}

// Reset 清除系统代理
func (p *linuxProxy) Reset() error {
	switch p.desktopType {
	case "gnome":
		return p.setGNOMEMode("none")
	case "kde":
		return p.applyKDE("", 0, "", false)
	}
	return nil
}

// GetCurrentProxy 获取当前系统代理配置
func (p *linuxProxy) GetCurrentProxy() (*ProxyInfo, error) {
	if p.desktopType != "gnome" || p.conn == nil {
		return &ProxyInfo{}, nil
	}

	obj := p.conn.Object("org.gnome.system.proxy", "/org/gnome/system/proxy")

	var mode string
	if err := obj.Call("org.freedesktop.DBus.Properties.Get", 0,
		"org.gnome.system.proxy", "mode").Store(&mode); err != nil {
		return nil, fmt.Errorf("get proxy mode: %w", err)
	}

	info := &ProxyInfo{Enabled: mode == "manual"}
	if mode == "manual" {
		httpObj := p.conn.Object("org.gnome.system.proxy", "/org/gnome/system/proxy/http")
		var host string
		var portV int32
		_ = httpObj.Call("org.freedesktop.DBus.Properties.Get", 0,
			"org.gnome.system.proxy.http", "host").Store(&host)
		_ = httpObj.Call("org.freedesktop.DBus.Properties.Get", 0,
			"org.gnome.system.proxy.http", "port").Store(&portV)
		info.Host = host
		info.Port = int(portV)
	}

	return info, nil
}

// ─── GNOME D-Bus 实现 ─────────────────────────────────────────────────────────

func (p *linuxProxy) applyGNOME(host string, port int, bypass string, pac bool, verge config.IVerge) error {
	if p.conn == nil {
		return fmt.Errorf("D-Bus connection not available")
	}

	if pac {
		// PAC 模式
		pacPort := 7890 // PAC server 端口（由 Wails HTTP server 提供）
		pacURL := fmt.Sprintf("http://%s:%d/commands/pac", host, pacPort)

		if err := p.setGNOMEAutoProxy(pacURL); err != nil {
			return err
		}
		return p.setGNOMEMode("auto")
	}

	// 手动代理模式
	if err := p.setGNOMEHTTPProxy(host, port); err != nil {
		return err
	}
	if err := p.setGNOMEHTTPSProxy(host, port); err != nil {
		return err
	}
	if err := p.setGNOMESocksProxy(host, port); err != nil {
		return err
	}
	if err := p.setGNOMEBypass(bypass); err != nil {
		return err
	}
	return p.setGNOMEMode("manual")
}

func (p *linuxProxy) setGNOMEMode(mode string) error {
	return p.setGNOMEProp("/org/gnome/system/proxy", "org.gnome.system.proxy", "mode", mode)
}

func (p *linuxProxy) setGNOMEAutoProxy(url string) error {
	return p.setGNOMEProp("/org/gnome/system/proxy", "org.gnome.system.proxy", "autoconfig-url", url)
}

func (p *linuxProxy) setGNOMEHTTPProxy(host string, port int) error {
	path := "/org/gnome/system/proxy/http"
	iface := "org.gnome.system.proxy.http"
	if err := p.setGNOMEProp(path, iface, "host", host); err != nil {
		return err
	}
	return p.setGNOMEProp(path, iface, "port", int32(port))
}

func (p *linuxProxy) setGNOMEHTTPSProxy(host string, port int) error {
	path := "/org/gnome/system/proxy/https"
	iface := "org.gnome.system.proxy.https"
	if err := p.setGNOMEProp(path, iface, "host", host); err != nil {
		return err
	}
	return p.setGNOMEProp(path, iface, "port", int32(port))
}

func (p *linuxProxy) setGNOMESocksProxy(host string, port int) error {
	path := "/org/gnome/system/proxy/socks"
	iface := "org.gnome.system.proxy.socks"
	if err := p.setGNOMEProp(path, iface, "host", host); err != nil {
		return err
	}
	return p.setGNOMEProp(path, iface, "port", int32(port))
}

func (p *linuxProxy) setGNOMEBypass(bypass string) error {
	// GNOME bypass 是字符串数组
	bypassList := []string{}
	start := 0
	for i := 0; i <= len(bypass); i++ {
		if i == len(bypass) || bypass[i] == ',' {
			item := bypass[start:i]
			if item != "" {
				bypassList = append(bypassList, item)
			}
			start = i + 1
		}
	}
	return p.setGNOMEProp("/org/gnome/system/proxy", "org.gnome.system.proxy",
		"ignore-hosts", bypassList)
}

func (p *linuxProxy) setGNOMEProp(path, iface, prop string, val interface{}) error {
	if p.conn == nil {
		return fmt.Errorf("D-Bus not connected")
	}
	obj := p.conn.Object("org.gnome.system.proxy", dbus.ObjectPath(path))
	call := obj.Call("org.freedesktop.DBus.Properties.Set", 0, iface, prop, dbus.MakeVariant(val))
	if call.Err != nil {
		utils.Log().Warn("D-Bus set property failed",
			zap.String("path", path),
			zap.String("prop", prop),
			zap.Error(call.Err))
		// fallback: 尝试 gsettings CLI
		return p.setGSetting(iface[len("org.gnome."):], prop, fmt.Sprintf("%v", val))
	}
	return nil
}

// setGSetting 通过 gsettings CLI 设置（D-Bus fallback）
func (p *linuxProxy) setGSetting(schema, key, value string) error {
	return exec.Command("gsettings", "set", schema, key, value).Run()
}

// ─── KDE 实现 ─────────────────────────────────────────────────────────────────

func (p *linuxProxy) applyKDE(host string, port int, bypass string, pac bool) error {
	kwrite := "kwriteconfig5"
	if _, err := exec.LookPath("kwriteconfig6"); err == nil {
		kwrite = "kwriteconfig6"
	}

	file := "--file"
	kioslaverc := "kioslaverc"

	if host == "" {
		// 禁用代理
		return exec.Command(kwrite, file, kioslaverc, "--group", "Proxy Settings",
			"--key", "ProxyType", "0").Run()
	}

	cmds := [][]string{
		{kwrite, file, kioslaverc, "--group", "Proxy Settings", "--key", "ProxyType", "1"},
		{kwrite, file, kioslaverc, "--group", "Proxy Settings", "--key", "httpProxy",
			fmt.Sprintf("http://%s %d", host, port)},
		{kwrite, file, kioslaverc, "--group", "Proxy Settings", "--key", "httpsProxy",
			fmt.Sprintf("http://%s %d", host, port)},
		{kwrite, file, kioslaverc, "--group", "Proxy Settings", "--key", "socksProxy",
			fmt.Sprintf("socks://%s %d", host, port)},
		{kwrite, file, kioslaverc, "--group", "Proxy Settings", "--key", "NoProxyFor", bypass},
	}

	for _, cmd := range cmds {
		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			utils.Log().Warn("KDE proxy set failed", zap.Strings("cmd", cmd), zap.Error(err))
		}
	}

	// 通知 KDE 刷新代理设置
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler",
		"org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:").Run()

	return nil
}

// portString 辅助函数
func portString(port int) string { return strconv.Itoa(port) }

// 确保 strconv 被使用
var _ = portString
