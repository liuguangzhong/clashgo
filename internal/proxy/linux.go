//go:build linux

package proxy

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

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
	// 优先检测 gsettings（GNOME / Cinnamon / MATE / Unity / Budgie 等）
	if _, err := exec.LookPath("gsettings"); err == nil {
		// 验证 schema 存在
		out, err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Output()
		if err == nil && len(out) > 0 {
			p.desktopType = "gnome"
			// 尝试连接 D-Bus（可选，用于更快的设置）
			conn, err := dbus.ConnectSessionBus()
			if err == nil {
				p.conn = conn
			}
			utils.Log().Info("Detected GNOME-compatible desktop (gsettings), using for system proxy")
			return
		}
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

	// fallback: 服务器/无桌面环境，通过写文件方式设置代理环境变量
	p.desktopType = "env"
	utils.Log().Warn("Unknown desktop environment, will set proxy via environment variables (/etc/environment + ~/.bashrc)")
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
		// 服务器/无桌面环境：写入环境变量文件
		return p.applyEnvProxy(host, port, bypass)
	}
}

// Reset 清除系统代理
func (p *linuxProxy) Reset() error {
	switch p.desktopType {
	case "gnome":
		return p.setGNOMEMode("none")
	case "kde":
		return p.applyKDE("", 0, "", false)
	default:
		return p.resetEnvProxy()
	}
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
	if p.conn != nil {
		obj := p.conn.Object("org.gnome.system.proxy", dbus.ObjectPath(path))
		call := obj.Call("org.freedesktop.DBus.Properties.Set", 0, iface, prop, dbus.MakeVariant(val))
		if call.Err == nil {
			return nil
		}
		utils.Log().Debug("D-Bus set property failed, falling back to gsettings",
			zap.String("prop", prop),
			zap.Error(call.Err))
	}
	// fallback: gsettings CLI（schema 即 iface 全名，如 org.gnome.system.proxy.http）
	var valStr string
	switch v := val.(type) {
	case []string:
		// gsettings 数组格式: "['item1', 'item2']"
		items := make([]string, len(v))
		for i, s := range v {
			items[i] = "'" + s + "'"
		}
		valStr = "[" + joinStrings(items, ", ") + "]"
	case int32:
		valStr = fmt.Sprintf("%d", v)
	default:
		valStr = fmt.Sprintf("'%v'", v)
	}
	return p.setGSetting(iface, prop, valStr)
}

// joinStrings 简单的字符串连接
func joinStrings(items []string, sep string) string {
	result := ""
	for i, s := range items {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// setGSetting 通过 gsettings CLI 设置
func (p *linuxProxy) setGSetting(schema, key, value string) error {
	cmd := exec.Command("gsettings", "set", schema, key, value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		utils.Log().Warn("gsettings set failed",
			zap.String("schema", schema),
			zap.String("key", key),
			zap.String("value", value),
			zap.String("output", string(out)),
			zap.Error(err))
	}
	return err
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

// ─── 服务器/无桌面环境：环境变量代理 ──────────────────────────────────────────

const (
	etcEnvironment   = "/etc/environment"
	proxyMarkerBegin = "# ClashGo proxy begin"
	proxyMarkerEnd   = "# ClashGo proxy end"
)

// applyEnvProxy 在服务器环境下设置代理环境变量
// 1. 写入 /etc/environment（需要 root，对所有用户全局有效，需重新登录/source）
// 2. 写入 ~/.bashrc 和 ~/.profile（当前用户，新 shell 自动生效）
// 3. 打印 export 命令，用户可直接复制到当前 shell 立即生效
func (p *linuxProxy) applyEnvProxy(host string, port int, bypass string) error {
	proxyURL := fmt.Sprintf("http://%s:%d", host, port)
	lines := []string{
		proxyMarkerBegin,
		fmt.Sprintf("http_proxy=%s", proxyURL),
		fmt.Sprintf("HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("https_proxy=%s", proxyURL),
		fmt.Sprintf("HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("all_proxy=%s", proxyURL),
		fmt.Sprintf("ALL_PROXY=%s", proxyURL),
		fmt.Sprintf("no_proxy=%s", bypass),
		fmt.Sprintf("NO_PROXY=%s", bypass),
		proxyMarkerEnd,
	}

	// 尝试写入 /etc/environment（需要 root 权限）
	if err := writeEnvBlock(etcEnvironment, lines); err != nil {
		utils.Log().Warn("Cannot write /etc/environment (need root?), trying user files",
			zap.Error(err))
	}

	// 写入用户 shell 配置（export 格式）
	exportLines := make([]string, 0, len(lines))
	exportLines = append(exportLines, proxyMarkerBegin)
	for _, l := range lines[1 : len(lines)-1] {
		exportLines = append(exportLines, "export "+l)
	}
	exportLines = append(exportLines, proxyMarkerEnd)

	home, err := os.UserHomeDir()
	if err == nil {
		for _, rcFile := range []string{".bashrc", ".profile", ".zshrc"} {
			path := filepath.Join(home, rcFile)
			if _, statErr := os.Stat(path); statErr == nil {
				_ = writeEnvBlock(path, exportLines)
			}
		}
	}

	// 打印可直接复制使用的 export 命令（在当前 shell 立即生效）
	utils.Log().Info("Proxy environment variables set. To apply in CURRENT shell, run:")
	utils.Log().Info(fmt.Sprintf("  export http_proxy=%s https_proxy=%s all_proxy=%s no_proxy='%s'",
		proxyURL, proxyURL, proxyURL, bypass))
	utils.Log().Info("Files updated: /etc/environment, ~/.bashrc, ~/.profile (if they exist)")
	utils.Log().Info("NOTE: New SSH sessions will inherit proxy automatically; current session needs manual export")
	return nil
}

// resetEnvProxy 清除服务器环境下的代理环境变量
func (p *linuxProxy) resetEnvProxy() error {
	if err := clearEnvBlock(etcEnvironment); err != nil {
		utils.Log().Warn("Cannot clear /etc/environment", zap.Error(err))
	}

	home, err := os.UserHomeDir()
	if err == nil {
		for _, rcFile := range []string{".bashrc", ".profile", ".zshrc"} {
			path := filepath.Join(home, rcFile)
			if _, statErr := os.Stat(path); statErr == nil {
				_ = clearEnvBlock(path)
			}
		}
	}
	utils.Log().Info("Proxy environment variables cleared from /etc/environment and shell rc files")
	return nil
}

// writeEnvBlock 在文件中写入/替换 ClashGo 代理块
// 若文件中已存在 marker 则替换，否则追加到末尾
func writeEnvBlock(path string, lines []string) error {
	// 读取原始内容
	origContent := ""
	if data, err := os.ReadFile(path); err == nil {
		origContent = string(data)
	}

	// 删除旧的 ClashGo 块
	cleaned := removeEnvBlock(origContent)

	// 追加新块
	newBlock := strings.Join(lines, "\n") + "\n"
	newContent := strings.TrimRight(cleaned, "\n") + "\n" + newBlock

	return os.WriteFile(path, []byte(newContent), 0644)
}

// clearEnvBlock 从文件中删除 ClashGo 代理块
func clearEnvBlock(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cleaned := removeEnvBlock(string(data))
	return os.WriteFile(path, []byte(cleaned), 0644)
}

// removeEnvBlock 从文本内容中移除 ClashGo 代理标记块
func removeEnvBlock(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var result []string
	inBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == proxyMarkerBegin {
			inBlock = true
			continue
		}
		if strings.TrimSpace(line) == proxyMarkerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}
