package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"clashgo/internal/config"
	"clashgo/internal/mihomo"
	"clashgo/internal/utils"

	"go.uber.org/zap"
)

// ProxyAPI 代理核心控制 API
// 所有操作通过 Mihomo HTTP REST API 实现（非 mock）
type ProxyAPI struct {
	mgr *config.Manager
}

func NewProxyAPI(mgr *config.Manager) *ProxyAPI {
	return &ProxyAPI{mgr: mgr}
}

// coreLifecycle 扩展接口：核心完整生命周期（app.go 注入）
var coreLifecycle interface {
	Start(ctx context.Context) error
	Stop() error
	UpdateConfig() error
	Restart() error
}

// SetCoreLifecycle 由 app.go 注入 core.Manager（同时满足所有接口）
func SetCoreLifecycle(cm interface {
	Start(ctx context.Context) error
	Stop() error
	UpdateConfig() error
	ForceUpdateConfig() error
	Restart() error
}) {
	coreLifecycle = cm
	// 向后兼容：同时更新 coreManagerRef 和 coreRestarter
	coreManagerRef = cm
	coreRestarter = cm
}

// StartCore 启动 Mihomo 代理核心
// 对应原: start_core
func (a *ProxyAPI) StartCore() error {
	if coreLifecycle == nil {
		return fmt.Errorf("core manager not initialized")
	}
	return coreLifecycle.Start(context.Background())
}

// StopCore 停止 Mihomo 代理核心
// 对应原: stop_core
func (a *ProxyAPI) StopCore() error {
	if coreLifecycle == nil {
		return fmt.Errorf("core manager not initialized")
	}
	return coreLifecycle.Stop()
}

// SyncTrayProxySelection 通知托盘更新代理选项显示
// 对应原: sync_tray_proxy_selection
// 前端调用此方法后，应用层通过 Wails EventsEmit 广播 "tray:sync-proxy"
// api 层无法直接访问 Wails context，返回事件名让 app.go 层发送
func (a *ProxyAPI) SyncTrayProxySelection() string {
	return "tray:sync-proxy" // app.go 监听并调用 trayMgr.UpdateMenu()
}

// mihomoClient 从当前配置获取已认证的 Mihomo 客户端
func (a *ProxyAPI) mihomoClient() (*mihomo.Client, error) {
	clash := a.mgr.GetClash()

	server := "127.0.0.1:9097"
	if ctrl, ok := clash["external-controller"].(string); ok && ctrl != "" {
		server = ctrl
	}

	secret := ""
	if s, ok := clash["secret"].(string); ok {
		secret = s
	}

	client := mihomo.NewClient(server, secret)
	if !client.IsAlive() {
		return nil, fmt.Errorf("Mihomo core is not running (server: %s)", server)
	}
	return client, nil
}

// GetProxies 获取所有代理节点及代理组
// 对应原: cmd::get_proxies
func (a *ProxyAPI) GetProxies() (*mihomo.ProxiesResponse, error) {
	utils.Log().Info("[链路] GetProxies 被调用")
	client, err := a.mihomoClient()
	if err != nil {
		utils.Log().Error("[链路] GetProxies mihomoClient 失败", zap.Error(err))
		return nil, err
	}
	resp, err := client.GetProxies(context.Background())
	if err != nil {
		utils.Log().Error("[链路] GetProxies HTTP 请求失败", zap.Error(err))
		return nil, err
	}
	proxyCount := 0
	groupCount := 0
	if resp != nil && resp.Proxies != nil {
		proxyCount = len(resp.Proxies)
		for _, p := range resp.Proxies {
			if len(p.All) > 0 {
				groupCount++
			}
		}
	}
	utils.Log().Info("[链路] GetProxies 成功",
		zap.Int("proxies", proxyCount),
		zap.Int("groups", groupCount))
	return resp, nil
}

// GetRules 获取当前规则列表
// 对应原: cmd::get_rules
func (a *ProxyAPI) GetRules() (*mihomo.RulesResponse, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return nil, err
	}
	return client.GetRules(context.Background())
}

// GetProviders 获取代理提供者列表
// 对应原: cmd::get_providers_proxies
func (a *ProxyAPI) GetProviders() (*mihomo.ProvidersResponse, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return nil, err
	}
	return client.GetProxyProviders(context.Background())
}

// SelectProxy 选择代理组中的节点
// 对应原: cmd::select_proxy
func (a *ProxyAPI) SelectProxy(group, proxyName string) error {
	client, err := a.mihomoClient()
	if err != nil {
		return err
	}
	return client.SelectProxy(context.Background(), group, proxyName)
}

// TestProxyDelay 测试单个节点延迟
func (a *ProxyAPI) TestProxyDelay(name, testURL string, timeout int) (uint16, error) {
	if testURL == "" {
		verge := a.mgr.GetVerge()
		if verge.DefaultLatencyTest != nil && *verge.DefaultLatencyTest != "" {
			testURL = *verge.DefaultLatencyTest
		} else {
			testURL = "https://www.gstatic.com/generate_204"
		}
	}
	if timeout <= 0 {
		timeout = 5000
	}

	client, err := a.mihomoClient()
	if err != nil {
		return 0, err
	}
	return client.TestDelay(context.Background(), name, testURL, timeout)
}

// UpdateProxyProvider 更新指定代理提供者
// 对应原: cmd::update_providers_proxies
func (a *ProxyAPI) UpdateProxyProvider(name string) error {
	client, err := a.mihomoClient()
	if err != nil {
		return err
	}
	return client.UpdateProxyProvider(context.Background(), name)
}

// GetConnections 获取当前活跃连接
// 对应原: cmd::get_connections
func (a *ProxyAPI) GetConnections() (*mihomo.ConnectionsResponse, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return nil, err
	}
	return client.GetConnections(context.Background())
}

// DeleteConnection 断开指定连接
// 对应原: cmd::delete_connection
func (a *ProxyAPI) DeleteConnection(id string) error {
	client, err := a.mihomoClient()
	if err != nil {
		return err
	}
	return client.CloseConnection(context.Background(), id)
}

// DeleteAllConnections 断开所有连接
// 对应原: cmd::delete_all_connections
func (a *ProxyAPI) DeleteAllConnections() error {
	client, err := a.mihomoClient()
	if err != nil {
		return err
	}
	return client.CloseAllConnections(context.Background())
}

// GetTraffic 获取当前流量统计
func (a *ProxyAPI) GetTraffic() (*mihomo.TrafficStats, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return nil, err
	}
	return client.GetTraffic(context.Background())
}

// GetCoreVersion 获取 Mihomo 核心版本
// 对应原: cmd::get_core_version
func (a *ProxyAPI) GetCoreVersion() (string, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return config.EmbeddedMihomoVersion, err
	}
	return client.GetVersion(context.Background())
}

// ChangeCoreVersion 切换并记录核心版本偏好（重启后生效）
// 对应原: cmd::change_clash_core
func (a *ProxyAPI) ChangeCoreVersion(coreName string) error {
	valid := false
	for _, v := range config.ValidClashCores {
		if v == coreName {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid core name: %q (valid: %v)", coreName, config.ValidClashCores)
	}
	patch := config.IVerge{ClashCore: &coreName}
	if err := a.mgr.PatchVerge(patch); err != nil {
		return err
	}
	if coreManagerRef != nil {
		return coreManagerRef.UpdateConfig()
	}
	return nil
}

// GetClashLogs 获取最近的内核日志快照
// 实时日志通过 WS /logs 流推送，此端点返回历史快照
func (a *ProxyAPI) GetClashLogs() []string {
	if coreManagerRef == nil {
		return nil
	}
	// 安全类型断言：只有实现了 GetLogs 方法的 core.Manager 才调用
	type logGetter interface{ GetLogs() []string }
	if lg, ok := coreManagerRef.(logGetter); ok {
		return lg.GetLogs()
	}
	return nil
}

// UpdateGeoData 触发 GeoIP/GeoSite 数据库更新
// 实现方式：由 Mihomo 在配置重载时自动下载新 Geo DB
// 步骤：1) 删除本地缓存 Geo 文件  2) 重载配置（触发 Mihomo 重新下载）
func (a *ProxyAPI) UpdateGeoData() error {
	// 删除当前 Geo 缓存文件，这样 Mihomo 重载时会重新下载
	geoFiles := []string{"geoip.dat", "geosite.dat", "geoip.metadb", "country.mmdb"}
	homeDir := utils.Dirs().HomeDir()
	for _, f := range geoFiles {
		_ = os.Remove(filepath.Join(homeDir, f))
	}

	// 重载配置触发 Mihomo 自动下载最新 GeoData
	client, err := a.mihomoClient()
	if err != nil {
		return err
	}
	runtimePath := getRuntimeConfigPath()
	return client.ReloadConfig(context.Background(), runtimePath)
}

// DNSQuery 通过 Mihomo 内嵌 DNS 解析
// 对应原: cmd::dns_query
func (a *ProxyAPI) DNSQuery(name, qtype string) ([]string, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return nil, err
	}
	return client.DNSQuery(context.Background(), name, qtype)
}

// RestartCore 重启代理核心
func (a *ProxyAPI) RestartCore() error {
	if coreRestarter == nil {
		return fmt.Errorf("core manager not initialized")
	}
	return coreRestarter.Restart()
}

// getRuntimeConfigPath 返回运行时配置文件完整路径
func getRuntimeConfigPath() string {
	return utils.Dirs().RuntimeConfigPath()
}

// ClearLogs 清除内核日志缓存
// 对应原: clear_logs
func (a *ProxyAPI) ClearLogs() {
	type logClearer interface{ ClearLogs() }
	if lc, ok := coreManagerRef.(logClearer); ok {
		lc.ClearLogs()
	}
}

// TestDelay 对指定 URL 做连通性测试（通过 Mihomo 代理）
// 对应原: test_delay
func (a *ProxyAPI) TestDelay(url string) (int, error) {
	client, err := a.mihomoClient()
	if err != nil {
		return -1, err
	}
	delay, err := client.TestDelay(context.Background(), "GLOBAL", url, 5000)
	if err != nil {
		return -1, err
	}
	return int(delay), nil
}
