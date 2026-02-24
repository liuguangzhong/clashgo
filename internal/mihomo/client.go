// Package mihomo 封装对 Mihomo 内核 RESTful API 的真实访问
// Mihomo API 文档: https://metacubex.github.io/Clash.Meta/api/
//
// 当 core/manager.go 以 Sidecar 模式启动 mihomo 进程后，
// 所有运行时数据（代理列表、连接、流量）均通过此客户端访问。

package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client Mihomo HTTP API 客户端（并发安全）
type Client struct {
	server     string // 例如 "127.0.0.1:9097"
	secret     string // Bearer token
	httpClient *http.Client
}

// NewClient 创建 Mihomo API 客户端
func NewClient(server, secret string) *Client {
	return &Client{
		server: server,
		secret: secret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ── 代理 Proxies ──────────────────────────────────────────────────────────────

// ProxiesResponse GET /proxies 响应
type ProxiesResponse struct {
	Proxies map[string]Proxy `json:"proxies"`
}

// Proxy 单个代理节点信息
type Proxy struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Alive   bool     `json:"alive"`
	History []Delay  `json:"history"`
	Now     string   `json:"now,omitempty"`
	All     []string `json:"all,omitempty"`
	UDP     bool     `json:"udp"`
}

// Delay 延迟记录
type Delay struct {
	Time  string `json:"time"`
	Delay uint16 `json:"delay"`
}

// GetProxies 获取所有代理及组信息
func (c *Client) GetProxies(ctx context.Context) (*ProxiesResponse, error) {
	var resp ProxiesResponse
	if err := c.get(ctx, "/proxies", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SelectProxy 选择代理组中的节点 PUT /proxies/:name
func (c *Client) SelectProxy(ctx context.Context, group, name string) error {
	body := map[string]string{"name": name}
	return c.put(ctx, "/proxies/"+group, body, nil)
}

// TestDelay 测试单个代理的延迟 GET /proxies/:name/delay
func (c *Client) TestDelay(ctx context.Context, proxyName, testURL string, timeoutMs int) (uint16, error) {
	path := fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%d",
		proxyName, urlEncode(testURL), timeoutMs)
	var resp struct {
		Delay uint16 `json:"delay"`
	}
	if err := c.get(ctx, path, &resp); err != nil {
		return 0, err
	}
	return resp.Delay, nil
}

// ── 规则 Rules ────────────────────────────────────────────────────────────────

// RulesResponse GET /rules 响应
type RulesResponse struct {
	Rules []Rule `json:"rules"`
}

// Rule 单条规则
type Rule struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
}

// GetRules 获取当前所有规则
func (c *Client) GetRules(ctx context.Context) (*RulesResponse, error) {
	var resp RulesResponse
	if err := c.get(ctx, "/rules", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ── 连接 Connections ──────────────────────────────────────────────────────────

// ConnectionsResponse GET /connections 响应
type ConnectionsResponse struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
}

// Connection 单条活跃连接
type Connection struct {
	ID          string         `json:"id"`
	Metadata    ConnectionMeta `json:"metadata"`
	Upload      int64          `json:"upload"`
	Download    int64          `json:"download"`
	Start       time.Time      `json:"start"`
	Chains      []string       `json:"chains"`
	Rule        string         `json:"rule"`
	RulePayload string         `json:"rulePayload"`
}

// ConnectionMeta 连接元信息
type ConnectionMeta struct {
	Network     string `json:"network"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	SourceIP    string `json:"sourceIP"`
	SourcePort  string `json:"sourcePort"`
	RemoteAddr  string `json:"remoteAddr"`
	DNSMode     string `json:"dnsMode"`
	ProcessPath string `json:"processPath"`
}

// GetConnections 获取所有活跃连接
func (c *Client) GetConnections(ctx context.Context) (*ConnectionsResponse, error) {
	var resp ConnectionsResponse
	if err := c.get(ctx, "/connections", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CloseConnection 关闭指定连接 DELETE /connections/:id
func (c *Client) CloseConnection(ctx context.Context, id string) error {
	return c.delete(ctx, "/connections/"+id)
}

// CloseAllConnections 关闭所有连接 DELETE /connections
func (c *Client) CloseAllConnections(ctx context.Context) error {
	return c.delete(ctx, "/connections")
}

// ── 代理提供者 Providers ──────────────────────────────────────────────────────

// ProvidersResponse GET /providers/proxies 响应
type ProvidersResponse struct {
	Providers map[string]Provider `json:"providers"`
}

// Provider 代理提供者
type Provider struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	VehicleType string  `json:"vehicleType"`
	Proxies     []Proxy `json:"proxies"`
	UpdatedAt   string  `json:"updatedAt"`
}

// GetProxyProviders 获取所有代理提供者
func (c *Client) GetProxyProviders(ctx context.Context) (*ProvidersResponse, error) {
	var resp ProvidersResponse
	if err := c.get(ctx, "/providers/proxies", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateProxyProvider 更新指定提供者 PUT /providers/proxies/:name
func (c *Client) UpdateProxyProvider(ctx context.Context, name string) error {
	return c.put(ctx, "/providers/proxies/"+name, nil, nil)
}

// ── 配置 Config ───────────────────────────────────────────────────────────────

// ReloadConfig 热加载配置文件 PUT /configs?force=true
func (c *Client) ReloadConfig(ctx context.Context, configPath string) error {
	body := map[string]string{"path": configPath}
	return c.put(ctx, "/configs?force=true", body, nil)
}

// GetVersion 获取 Mihomo 版本
func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var resp struct {
		Version string `json:"version"`
		Meta    bool   `json:"meta"`
	}
	if err := c.get(ctx, "/version", &resp); err != nil {
		return "", err
	}
	return resp.Version, nil
}

// ── 流量统计 Traffic ──────────────────────────────────────────────────────────

// TrafficStats GET /traffic 瞬时流速（仅一帧，非流式）
type TrafficStats struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// GetTraffic 获取当前流量速率
func (c *Client) GetTraffic(ctx context.Context) (*TrafficStats, error) {
	var stats TrafficStats
	if err := c.get(ctx, "/traffic", &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// ── DNS ───────────────────────────────────────────────────────────────────────

// DNSQuery 通过 Mihomo DNS 解析域名 GET /dns/query?name=xxx&type=A
func (c *Client) DNSQuery(ctx context.Context, name, qtype string) ([]string, error) {
	path := fmt.Sprintf("/dns/query?name=%s&type=%s", name, qtype)
	var resp struct {
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	answers := make([]string, len(resp.Answer))
	for i, a := range resp.Answer {
		answers[i] = a.Data
	}
	return answers, nil
}

// IsAlive 检查 Mihomo 是否在线
func (c *Client) IsAlive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.GetVersion(ctx)
	return err == nil
}

// ── HTTP 基础方法 ─────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+c.server+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) put(ctx context.Context, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"http://"+c.server+path, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, out)
}

func (c *Client) delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"http://"+c.server+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

func (c *Client) do(req *http.Request, out interface{}) error {
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mihomo API %s %s: HTTP %d %s",
			req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// urlEncode 对单个查询参数值进行 percent-encoding，避免破坏查询字符串
func urlEncode(s string) string {
	return url.QueryEscape(s)
}
