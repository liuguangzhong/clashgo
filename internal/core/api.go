package core

// api.go — Mihomo HTTP API fallback
//
// Clash Verge Rev 原版（Rust）通过 tauri-plugin-mihomo 的 HTTP 客户端
// 调用 Mihomo 子进程的 RESTful API（见原版 reload_config / update_config）。
//
// ClashGo 采用同进程嵌入，正常路径下直接调用 executor.ApplyConfig，
// 无需 HTTP 通信。但保留此文件作为：
//   1. 调试/开发时可用 external-controller 地址直接操控内核
//   2. 未来支持远程 Mihomo 实例的兼容入口

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// mihomoClient 封装对 Mihomo RESTful API 的 HTTP 访问
// 对应原版 tauri-plugin-mihomo 的客户端实现
type mihomoClient struct {
	server     string // "host:port"，如 "127.0.0.1:9097"
	secret     string // Bearer token
	httpClient *http.Client
}

func newMihomoClient(server, secret string) *mihomoClient {
	return &mihomoClient{
		server: server,
		secret: secret,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// reloadConfig 热加载配置（对应原版 reload_config → PUT /configs?force=true）
//
// Clash Verge Rev 原版：
//
//	handle::Handle::mihomo().await.reload_config(true, path).await
//
// 这里的 force=true 对应 PUT /configs?force=true，
// 等价于 hub.ApplyConfig（强制重建 Listener）。
func (c *mihomoClient) reloadConfig(ctx context.Context, configPath string) error {
	payload, _ := json.Marshal(map[string]string{"path": configPath})
	url := fmt.Sprintf("http://%s/configs?force=true", c.server)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create reload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("reload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("reload failed: HTTP %d %s", resp.StatusCode, string(body))
}

// getVersion 获取 Mihomo 版本（连通性检查）
func (c *mihomoClient) getVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://%s/version", c.server), nil)
	if err != nil {
		return "", err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("version request: %w", err)
	}
	defer resp.Body.Close()

	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// isAlive 检查 Mihomo external-controller 是否可达
func (c *mihomoClient) isAlive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.getVersion(ctx)
	return err == nil
}

// patchMode 切换代理模式（rule / global / direct）
// 对应 PATCH /configs 接口
func (c *mihomoClient) patchMode(ctx context.Context, mode string) error {
	payload, _ := json.Marshal(map[string]string{"mode": mode})
	url := fmt.Sprintf("http://%s/configs", c.server)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("patch mode: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("patch mode failed: HTTP %d %s", resp.StatusCode, string(body))
}

// reloadMihomoConfig 包级别便捷函数（向后兼容，内部委托给 mihomoClient）
func reloadMihomoConfig(server, secret, configPath string) error {
	c := newMihomoClient(server, secret)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return c.reloadConfig(ctx, configPath)
}
