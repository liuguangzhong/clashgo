package proxy

// doh.go — DNS over HTTPS 上游支持（P2）
//
// 参照 mihomo/dns/doh.go 实现。
// 支持：
//   - RFC 8484 DoH（application/dns-message）
//   - GET 和 POST 方式
//   - 配置格式：https://dns.google/dns-query

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DoHClient DoH DNS 客户端（对应 mihomo/dns.dohClient）
type DoHClient struct {
	url        string
	httpClient *http.Client
}

func NewDoHClient(url string) *DoHClient {
	return &DoHClient{
		url: url,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Exchange 发送 DNS 查询并返回响应（对应 mihomo dohClient.ExchangeContext）
func (c *DoHClient) Exchange(ctx context.Context, query []byte) ([]byte, error) {
	// RFC 8484 GET 格式：?dns=<base64url>
	b64 := base64.RawURLEncoding.EncodeToString(query)
	reqURL := c.url
	if strings.Contains(reqURL, "?") {
		reqURL += "&dns=" + b64
	} else {
		reqURL += "?dns=" + b64
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-message")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH GET %s: %w", c.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH response status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ExchangePOST 使用 POST 方式发送 DoH 查询（对应 mihomo dohClient POST 分支）
func (c *DoHClient) ExchangePOST(ctx context.Context, query []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DoH POST %s: %w", c.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH response status: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// QueryDoH 查询域名 A 记录（通过 DoH）
func QueryDoH(ctx context.Context, url, host string) ([]net.IP, time.Duration, error) {
	query := buildDNSQuery(host)
	client := NewDoHClient(url)
	resp, err := client.Exchange(ctx, query)
	if err != nil {
		// POST fallback
		resp, err = client.ExchangePOST(ctx, query)
		if err != nil {
			return nil, 0, err
		}
	}
	return parseDNSResponse(resp)
}

// ── DoH 整合到 Resolver ───────────────────────────────────────────────────────

// ResolverWithDoH 支持 DoH 的扩展 Resolver
type ResolverWithDoH struct {
	*Resolver
	dohURLs []string
}

// NewResolverWithDoH 创建支持 DoH 的 DNS 解析器
func NewResolverWithDoH(nameservers, dohURLs []string, fakeIPSubnet string) (*ResolverWithDoH, error) {
	r, err := NewResolver(nameservers, fakeIPSubnet)
	if err != nil {
		return nil, err
	}
	return &ResolverWithDoH{Resolver: r, dohURLs: dohURLs}, nil
}

// Resolve 优先使用 DoH，失败则回退到 UDP DNS
func (r *ResolverWithDoH) Resolve(ctx context.Context, host string) (net.IP, error) {
	// 先尝试 DoH
	for _, url := range r.dohURLs {
		ips, _, err := QueryDoH(ctx, url, host)
		if err == nil && len(ips) > 0 {
			return ips[0], nil
		}
	}
	// 回退普通 UDP DNS
	return r.Resolver.Resolve(ctx, host)
}

// kernelDNSConfigExt 扩展 KernelConfig DNS 支持 DoH
// （将在 kernel.ApplyConfig 中使用）
type kernelDNSConfigExt = kernelDNSConfig
