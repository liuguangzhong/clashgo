package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// reloadMihomoConfig 通过 Mihomo HTTP API 热加载配置
// PUT /configs?force=true
func reloadMihomoConfig(server, secret, configPath string) error {
	payload, _ := json.Marshal(map[string]string{"path": configPath})

	req, err := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("http://%s/configs?force=true", server),
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload failed: HTTP %d", resp.StatusCode)
	}

	return nil
}
