package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"clashgo/internal/mihomo"
	"clashgo/internal/proxy"
	"clashgo/internal/utils"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
)

// SystemAPI 系统信息与操作 API
type SystemAPI struct {
	sysProxy  proxy.SysProxy
	mihomoSrv string
	secret    string
}

func NewSystemAPI() *SystemAPI {
	return &SystemAPI{
		sysProxy:  proxy.NewSysProxy(),
		mihomoSrv: "127.0.0.1:9097",
	}
}

func (a *SystemAPI) SetMihomoServer(server, secret string) {
	a.mihomoSrv = server
	a.secret = secret
}

func (a *SystemAPI) SetProxy(sp proxy.SysProxy) {
	a.sysProxy = sp
}

// FetchViaProxy 通过 Clash 代理端口（localhost:proxyPort）发起 HTTP GET，
// 返回响应体字符串。这是为了绕过 Linux 下 WebKit2GTK 的 fetch 不走系统代理的限制。
// 前端 IP 信息卡片通过 Wails IPC 调用此方法，而不是直接用浏览器 fetch。
func (a *SystemAPI) FetchViaProxy(targetURL string, proxyPort int) (string, error) {
	if proxyPort <= 0 {
		proxyPort = 7897
	}
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", proxyPort)
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return "", fmt.Errorf("invalid proxy address: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		return "", fmt.Errorf("fetch via proxy: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return string(body), nil
}

// ─── 系统代理 ─────────────────────────────────────────────────────────────────

// GetSysProxy 读取当前系统代理设置
func (a *SystemAPI) GetSysProxy() (*proxy.ProxyInfo, error) {
	return a.sysProxy.GetCurrentProxy()
}

// GetAutoProxy 返回与原项目兼容的代理信息格式
func (a *SystemAPI) GetAutoProxy() (map[string]interface{}, error) {
	info, err := a.sysProxy.GetCurrentProxy()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"enable": info.Enabled,
		"server": fmt.Sprintf("%s:%d", info.Host, info.Port),
		"bypass": info.Bypass,
	}, nil
}

// ─── 网络接口 ─────────────────────────────────────────────────────────────────

// NetworkInterface 网络接口信息
type NetworkInterface struct {
	Name      string   `json:"name"`
	Index     int      `json:"index"`
	MTU       int      `json:"mtu"`
	Flags     string   `json:"flags"`
	Addresses []string `json:"addresses"`
}

// GetNetworkInterfaces 获取所有网络接口（含地址）
func (a *SystemAPI) GetNetworkInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	result := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		ni := NetworkInterface{
			Name:  iface.Name,
			Index: iface.Index,
			MTU:   iface.MTU,
			Flags: iface.Flags.String(),
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ni.Addresses = append(ni.Addresses, addr.String())
		}
		result = append(result, ni)
	}
	return result, nil
}

// GetSystemHostname 获取主机名
// 对应原: get_system_hostname
func (a *SystemAPI) GetSystemHostname() (string, error) {
	return os.Hostname()
}

// ─── 系统信息 ─────────────────────────────────────────────────────────────────

// SystemInfo CPU/内存/系统信息
type SystemInfo struct {
	CPU        float64 `json:"cpu"`
	CPUCount   int     `json:"cpu_count"`
	MemTotal   uint64  `json:"mem_total"`
	MemUsed    uint64  `json:"mem_used"`
	MemAvail   uint64  `json:"mem_avail"`
	MemPercent float64 `json:"mem_percent"`
	OS         string  `json:"os"`
	Arch       string  `json:"arch"`
}

// GetSystemInfo 获取 CPU/内存实时信息
func (a *SystemAPI) GetSystemInfo() (*SystemInfo, error) {
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil {
		return nil, fmt.Errorf("get cpu percent: %w", err)
	}
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("get memory info: %w", err)
	}
	cpuCount, _ := cpu.Counts(true)
	return &SystemInfo{
		CPU:        cpuPercent[0],
		CPUCount:   cpuCount,
		MemTotal:   memStat.Total,
		MemUsed:    memStat.Used,
		MemAvail:   memStat.Available,
		MemPercent: memStat.UsedPercent,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}, nil
}

// ─── Mihomo 连接（系统层）────────────────────────────────────────────────────

func (a *SystemAPI) GetConnections() (*mihomo.ConnectionsResponse, error) {
	return mihomo.NewClient(a.mihomoSrv, a.secret).GetConnections(context.Background())
}

func (a *SystemAPI) DeleteConnection(id string) error {
	return mihomo.NewClient(a.mihomoSrv, a.secret).CloseConnection(context.Background(), id)
}

func (a *SystemAPI) DeleteAllConnections() error {
	return mihomo.NewClient(a.mihomoSrv, a.secret).CloseAllConnections(context.Background())
}

// ─── 流量统计 ─────────────────────────────────────────────────────────────────

// IOCounter 网络 I/O 计数器
type IOCounter struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
}

// GetTrafficStats 获取每个网络接口的实时 I/O 统计
func (a *SystemAPI) GetTrafficStats() ([]IOCounter, error) {
	counters, err := psnet.IOCounters(true)
	if err != nil {
		return nil, fmt.Errorf("get io counters: %w", err)
	}
	result := make([]IOCounter, len(counters))
	for i, c := range counters {
		result[i] = IOCounter{
			Name:        c.Name,
			BytesSent:   c.BytesSent,
			BytesRecv:   c.BytesRecv,
			PacketsSent: c.PacketsSent,
			PacketsRecv: c.PacketsRecv,
		}
	}
	return result, nil
}

// ─── 端口 ────────────────────────────────────────────────────────────────────

// CheckPortAvailable 检查端口是否空闲（TCP bind 测试）
// 对应原: is_port_in_use（返回值语义相反）
func (a *SystemAPI) CheckPortAvailable(port uint16) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// ─── 版本 ─────────────────────────────────────────────────────────────────────

func (a *SystemAPI) GetCoreVersion() (string, error) {
	return mihomo.NewClient(a.mihomoSrv, a.secret).GetVersion(context.Background())
}

// ─── 自启动 ───────────────────────────────────────────────────────────────────

// GetAutoLaunchStatus 获取当前开机自启状态
// 对应原: get_auto_launch_status
func (a *SystemAPI) GetAutoLaunchStatus() bool {
	return utils.NewAutoStart().IsEnabled()
}

// ─── 应用目录 ─────────────────────────────────────────────────────────────────

// GetAppDir 返回应用数据目录路径
// 对应原: get_app_dir
func (a *SystemAPI) GetAppDir() string {
	return utils.Dirs().HomeDir()
}

// GetPortableFlag 返回是否为便携模式
// 对应原: get_portable_flag
func (a *SystemAPI) GetPortableFlag() bool {
	return utils.Dirs().IsPortable()
}

// ─── 文件/目录 ────────────────────────────────────────────────────────────────

// OpenAppDir 在系统文件管理器中打开应用数据目录
func (a *SystemAPI) OpenAppDir() error {
	return openPath(utils.Dirs().HomeDir())
}

// OpenLogsDir 打开日志目录
func (a *SystemAPI) OpenLogsDir() error {
	return openPath(utils.Dirs().LogsDir())
}

// OpenCoreDir 打开可执行文件目录
func (a *SystemAPI) OpenCoreDir() error {
	dir, err := executableDir()
	if err != nil {
		return err
	}
	return openPath(dir)
}

// OpenAppLog 在系统默认编辑器中打开最新应用日志
// 对应原: open_app_log
func (a *SystemAPI) OpenAppLog() error {
	logPath := utils.Dirs().LatestLogPath()
	return openFile(logPath)
}

// OpenCoreLog 打开最新核心（Mihomo）日志
// 对应原: open_core_log
func (a *SystemAPI) OpenCoreLog() error {
	logPath := utils.Dirs().LatestCoreLogPath()
	return openFile(logPath)
}

// OpenWebURL 在系统默认浏览器中打开 URL
// 对应原: open_web_url
func (a *SystemAPI) OpenWebURL(url string) error {
	return openURL(url)
}

// ViewProfileFile 在系统文件管理器/编辑器中打开配置文件
// 对应原: view_profile
func (a *SystemAPI) ViewProfileFile(filePath string) error {
	return openFile(filePath)
}

// ─── 代理环境变量 ──────────────────────────────────────────────────────────────

// CopyClashEnv 生成代理环境变量文本（由前端复制到剪贴板）
// 对应原: copy_clash_env
// 命名为一致性保留；内部读取 clash 配置端口，不依赖系统代理是否开启
// Note: 推荐使用 ConfigAPI.CopyClashEnv ，这里是为了兼容老调用方式
func (a *SystemAPI) CopyClashEnv() string {
	// 读取配置端口（verge 优先，否则默认 17897）
	mixedPort := 17897
	// 通过 mihomoSrv 不能反向拿到 mgr，这里直接输出默认地址
	// 实际端口取决于配置，推荐前端调用 ConfigAPI.CopyClashEnv
	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", mixedPort)
	return fmt.Sprintf(
		"export https_proxy=%s\nexport http_proxy=%s\nexport all_proxy=%s\n",
		proxyAddr, proxyAddr, proxyAddr,
	)
}

// ─── 轻量模式 ─────────────────────────────────────────────────────────────────

// EntryLightweightMode 进入轻量后台模式
// 对应原: entry_lightweight_mode
// NOTE: 实际窗口隐藏由 App.EntryLightweightMode（app.go）实现，
// 前端应直接调用 App.EntryLightweightMode 而不是此方法。
// 此方法返回 trigger 事件名供前端二次触发。
func (a *SystemAPI) EntryLightweightMode() string {
	return "lightweight:enter"
}

// ExitLightweightMode 退出轻量模式
// 对应原: exit_lightweight_mode
// NOTE: 实际窗口显示由 App.ExitLightweightMode（app.go）实现。
func (a *SystemAPI) ExitLightweightMode() string {
	return "lightweight:exit"
}

// ─── 内部帮助函数 ─────────────────────────────────────────────────────────────

// openPath 用系统文件管理器打开目录
func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("explorer", path)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// openFile 用系统默认应用打开文件（编辑器/查看器）
func openFile(path string) error {
	// xdg-open / open / start 均支持文件和目录
	return openPath(path)
}

// openURL 在系统默认浏览器打开 URL
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// executableDir 返回当前可执行文件所在目录
func executableDir() (string, error) {
	return getExecDir()
}
