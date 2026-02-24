package api

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchURLBytes 下载 URL 内容，返回字节
func fetchURLBytes(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
