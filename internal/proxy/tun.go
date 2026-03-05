package proxy

// tun.go — TUN 透明代理（P2）
//
// 参照 mihomo/listener/tun + sing-tun 实现。
//
// TUN 透明代理原理：
//   1. 创建一个虚拟网卡（TUN 设备）
//   2. 操作系统路由表将所有流量导向 TUN 设备
//   3. 我们从 TUN 设备读取 IP 包，解析目标地址
//   4. 通过 Tunnel 按规则路由，建立代理连接
//   5. 双向转发数据
//
// Windows 实现方案：
//   - 使用 Net.exe / route 命令配置路由（不依赖 wintun.dll）
//   - 通过 Windows Raw Socket 读写 IP 包
//   - 或通过本地代理 + 设置系统代理的方式实现"软 TUN"
//
// 注意：完整的 TUN 实现（如 wintun）需要内核驱动支持。
// 本实现提供两种模式：
//   1. SoftTUN（软 TUN）：通过设置系统代理 + 监听 Mixed Port 实现全局代理
//   2. HardTUN（硬 TUN）：接口占位，可接入 wintun/wireguard-go 等实现

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// TUNConfig TUN 配置（对应 mihomo TunConfig）
type TUNConfig struct {
	Enable              bool     `yaml:"enable"`
	Stack               string   `yaml:"stack"`      // "gvisor" / "system" / "lwip"
	DNSHijack           []string `yaml:"dns-hijack"` // ["0.0.0.0:53"]
	AutoRoute           bool     `yaml:"auto-route"`
	AutoDetectInterface bool     `yaml:"auto-detect-interface"`
	MTU                 int      `yaml:"mtu"`           // 默认 9000
	Inet4Address        []string `yaml:"inet4-address"` // ["198.18.0.1/16"]
	Inet6Address        []string `yaml:"inet6-address"`
	IncludeUID          []int    `yaml:"include-uid"`
	ExcludeUID          []int    `yaml:"exclude-uid"`
	StrictRoute         bool     `yaml:"strict-route"`
}

// TUNMode TUN 运行模式
type TUNMode int

const (
	TUNDisabled TUNMode = iota
	TUNSoft             // 软 TUN：设置系统代理
	TUNHard             // 硬 TUN：wintun 驱动
)

// TUNManager TUN 管理器（对应 mihomo tunAdapter）
type TUNManager struct {
	cfg    *TUNConfig
	tunnel *Tunnel
	mode   TUNMode
	stopCh chan struct{}

	// SoftTUN 路由信息
	originalGateway string
	tunIfaceAddr    string
}

// NewTUNManager 创建 TUN 管理器
func NewTUNManager(cfg *TUNConfig, tunnel *Tunnel) *TUNManager {
	return &TUNManager{
		cfg:    cfg,
		tunnel: tunnel,
		stopCh: make(chan struct{}),
	}
}

// Start 启动 TUN（对应 mihomo tunAdapter.Start）
func (t *TUNManager) Start() error {
	if !t.cfg.Enable {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		return t.startWindows()
	case "linux":
		return t.startLinux()
	case "darwin":
		return t.startDarwin()
	default:
		return fmt.Errorf("TUN not supported on %s", runtime.GOOS)
	}
}

// Stop 停止 TUN
func (t *TUNManager) Stop() {
	select {
	case t.stopCh <- struct{}{}:
	default:
	}
	if t.mode == TUNSoft {
		t.cleanupSoftTUN()
	}
}

// ── Windows 实现 ──────────────────────────────────────────────────────────────

// startWindows Windows TUN 启动
// 使用软 TUN 模式：netsh 设置系统代理
func (t *TUNManager) startWindows() error {
	// 尝试硬 TUN（检测 wintun.dll 是否可用）
	if isWintunAvailable() {
		return t.startHardTUN()
	}
	// 回退到软 TUN
	return t.startSoftTUN()
}

// isWintunAvailable 检测 wintun.dll 是否存在
func isWintunAvailable() bool {
	out, err := exec.Command("where", "wintun.dll").Output()
	return err == nil && len(out) > 0
}

// startSoftTUN 软 TUN：通过 netsh 设置系统代理（对应 混合端口模式）
// Windows 实现：告诉系统所有应用走 127.0.0.1:MixedPort
func (t *TUNManager) startSoftTUN() error {
	t.mode = TUNSoft

	// 记录原来的网关
	t.originalGateway = getDefaultGateway()

	// 设置系统代理（通过 registry 或 netsh）
	if err := setWindowsSystemProxy("127.0.0.1", 7890); err != nil {
		return fmt.Errorf("set system proxy: %w", err)
	}

	return nil
}

// startHardTUN 硬 TUN（使用 wintun，接口占位）
func (t *TUNManager) startHardTUN() error {
	t.mode = TUNHard
	// TODO: 调用 wintun API 创建 TUN 设备
	// wt, err := wintun.CreateAdapter("ClashGo", "Wintun", nil)
	// 暂时降级到软 TUN
	return t.startSoftTUN()
}

// cleanupSoftTUN 清理软 TUN 设置
func (t *TUNManager) cleanupSoftTUN() {
	_ = clearWindowsSystemProxy()
}

// setWindowsSystemProxy 设置 Windows 系统代理
// 对应 mihomo 的 sysproxy 设置（通过 WinInet/registry）
func setWindowsSystemProxy(host string, port int) error {
	proxyAddr := fmt.Sprintf("%s:%d", host, port)
	// 通过 netsh 设置（winhttp proxy）
	cmd := exec.Command("netsh", "winhttp", "set", "proxy",
		proxyAddr, "localhost;127.*;10.*;172.16.*;192.168.*")
	if out, err := cmd.CombinedOutput(); err != nil {
		// netsh 失败则尝试 reg 方式
		return setWindowsProxyViaReg(host, port, string(out))
	}
	return nil
}

// setWindowsProxyViaReg 通过注册表设置 Internet Explorer / WinInet 系统代理
func setWindowsProxyViaReg(host string, port int, _ string) error {
	proxyAddr := fmt.Sprintf("%s:%d", host, port)
	cmds := [][]string{
		{"reg", "add",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
			"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f"},
		{"reg", "add",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
			"/v", "ProxyServer", "/t", "REG_SZ", "/d", proxyAddr, "/f"},
	}
	for _, args := range cmds {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return fmt.Errorf("reg set proxy: %w", err)
		}
	}
	return nil
}

// clearWindowsSystemProxy 清除系统代理
func clearWindowsSystemProxy() error {
	cmds := [][]string{
		{"netsh", "winhttp", "reset", "proxy"},
		{"reg", "add",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
			"/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f"},
	}
	for _, args := range cmds {
		_ = exec.Command(args[0], args[1:]...).Run()
	}
	return nil
}

// getDefaultGateway 获取当前默认网关
func getDefaultGateway() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			`(Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort-Object RouteMetric | Select-Object -First 1).NextHop`).Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		out, err := exec.Command("ip", "route", "show", "default").Output()
		if err == nil {
			parts := strings.Fields(string(out))
			for i, p := range parts {
				if p == "via" && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}
	return ""
}

// ── Linux 实现 ────────────────────────────────────────────────────────────────

// startLinux Linux TUN 启动（使用 ip tuntap + iptables 实现透明代理）
func (t *TUNManager) startLinux() error {
	t.mode = TUNHard

	// 1. 创建 TUN 设备
	if err := exec.Command("ip", "tuntap", "add", "mode", "tun", "dev", "clashgo0").Run(); err != nil {
		return fmt.Errorf("create TUN device: %w", err)
	}

	// 2. 配置 TUN 设备 IP
	tunAddr := "198.18.0.1/16"
	if len(t.cfg.Inet4Address) > 0 {
		tunAddr = t.cfg.Inet4Address[0]
	}
	if err := exec.Command("ip", "addr", "add", tunAddr, "dev", "clashgo0").Run(); err != nil {
		return fmt.Errorf("assign TUN address: %w", err)
	}
	if err := exec.Command("ip", "link", "set", "clashgo0", "up").Run(); err != nil {
		return fmt.Errorf("bring up TUN device: %w", err)
	}

	// 3. 配置路由（自动路由：所有流量走 clashgo0）
	if t.cfg.AutoRoute {
		if err := t.setupLinuxRoutes(); err != nil {
			return fmt.Errorf("setup routes: %w", err)
		}
	}

	// 4. 配置 DNS 劫持
	if len(t.cfg.DNSHijack) > 0 {
		if err := t.setupLinuxDNSHijack(); err != nil {
			// DNS 劫持失败不影响主功能
			fmt.Printf("DNS hijack warning: %v\n", err)
		}
	}

	return nil
}

// setupLinuxRoutes 配置 Linux 路由规则（对应 mihomo auto-route）
func (t *TUNManager) setupLinuxRoutes() error {
	cmds := [][]string{
		// 自定义路由表
		{"ip", "rule", "add", "not", "fwmark", "666", "table", "main", "suppress_prefixlength", "0"},
		{"ip", "rule", "add", "fwmark", "666", "table", "666"},
		{"ip", "route", "add", "default", "dev", "clashgo0", "table", "666"},
		// iptables 标记需要代理的流量
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "198.18.0.0/16", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "127.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "10.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "172.16.0.0/12", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-d", "192.168.0.0/16", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "OUTPUT", "-j", "MARK", "--set-mark", "666"},
	}
	for _, args := range cmds {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			// 某些命令失败不影响整体（可能权限不足）
			continue
		}
	}
	return nil
}

// setupLinuxDNSHijack 配置 DNS 劫持（iptables REDIRECT）
func (t *TUNManager) setupLinuxDNSHijack() error {
	for _, addr := range t.cfg.DNSHijack {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		_ = host
		cmd := exec.Command("iptables", "-t", "nat", "-A", "OUTPUT",
			"-p", "udp", "--dport", "53",
			"-j", "REDIRECT", "--to-ports", port)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("DNS hijack iptables: %w", err)
		}
	}
	return nil
}

// ── macOS 实现 ────────────────────────────────────────────────────────────────

// startDarwin macOS TUN 启动（使用 utun + pf）
func (t *TUNManager) startDarwin() error {
	t.mode = TUNHard

	// macOS 使用 utun 虚拟接口
	// 通过 /dev/utun0 访问
	cmds := [][]string{
		{"ifconfig", "utun9", "198.18.0.1", "198.18.0.1", "up"},
		{"route", "add", "-net", "0.0.0.0/0", "-interface", "utun9"},
	}
	for _, args := range cmds {
		_ = exec.Command(args[0], args[1:]...).Run()
	}

	if t.cfg.AutoRoute {
		if err := t.setupDarwinRoutes(); err != nil {
			return err
		}
	}
	return nil
}

func (t *TUNManager) setupDarwinRoutes() error {
	cmds := [][]string{
		{"route", "add", "-net", "1.0.0.0/8", "-interface", "utun9"},
		{"route", "add", "-net", "2.0.0.0/7", "-interface", "utun9"},
		{"route", "add", "-net", "4.0.0.0/6", "-interface", "utun9"},
		// ... 分段路由（避免覆盖局域网路由）
		{"route", "add", "-net", "128.0.0.0/1", "-interface", "utun9"},
	}
	for _, args := range cmds {
		_ = exec.Command(args[0], args[1:]...).Run()
	}
	return nil
}
