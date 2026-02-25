package geodata

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"clashgo/internal/utils"

	"go.uber.org/zap"
)

// GeoFile 需要下载的 geo 数据文件
type GeoFile struct {
	Name    string   // 文件名
	Mirrors []string // 多镜像 URL（按优先级排序）
}

// DefaultGeoFiles 返回需要的 geo 数据文件及其多镜像下载地址
func DefaultGeoFiles() []GeoFile {
	return []GeoFile{
		{
			Name: "geoip.metadb",
			Mirrors: []string{
				"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
				"https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
				"https://ghfast.top/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
				"https://mirror.ghproxy.com/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb",
			},
		},
		{
			Name: "geosite.dat",
			Mirrors: []string{
				"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
				"https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
				"https://ghfast.top/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
				"https://mirror.ghproxy.com/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
			},
		},
		{
			Name: "country.mmdb",
			Mirrors: []string{
				"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
				"https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
				"https://ghfast.top/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
				"https://mirror.ghproxy.com/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
			},
		},
		{
			Name: "GeoLite2-ASN.mmdb",
			Mirrors: []string{
				"https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
				"https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
				"https://ghfast.top/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb",
				"https://mirror.ghproxy.com/https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb",
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/GeoLite2-ASN.mmdb",
			},
		},
	}
}

// EnsureGeoData 确保所有 geo 数据文件存在于 homeDir 中
// 如果文件不存在，会依次尝试多个镜像下载
func EnsureGeoData(homeDir string) {
	log := utils.Log()
	files := DefaultGeoFiles()

	for _, gf := range files {
		destPath := filepath.Join(homeDir, gf.Name)

		// 文件已存在且大小合理，跳过
		if info, err := os.Stat(destPath); err == nil && info.Size() > 1024 {
			log.Debug("Geo data file exists", zap.String("file", gf.Name), zap.Int64("size", info.Size()))
			continue
		}

		log.Info("Downloading geo data file", zap.String("file", gf.Name))

		var lastErr error
		downloaded := false

		for i, mirror := range gf.Mirrors {
			log.Info("Trying mirror",
				zap.String("file", gf.Name),
				zap.Int("mirror", i+1),
				zap.Int("total", len(gf.Mirrors)),
				zap.String("url", truncateURL(mirror)),
			)

			if err := downloadFile(mirror, destPath); err != nil {
				lastErr = err
				log.Warn("Mirror failed, trying next",
					zap.String("file", gf.Name),
					zap.Int("mirror", i+1),
					zap.Error(err),
				)
				continue
			}

			downloaded = true
			log.Info("Geo data downloaded successfully",
				zap.String("file", gf.Name),
				zap.Int("mirror", i+1),
			)
			break
		}

		if !downloaded {
			log.Error("All mirrors failed for geo data file",
				zap.String("file", gf.Name),
				zap.Error(lastErr),
			)
		}
	}
}

// downloadFile 下载文件到指定路径，带超时和重试
func downloadFile(url, destPath string) error {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	// 写入临时文件，成功后 rename（原子操作）
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	written, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download write: %w", err)
	}

	if written < 1024 {
		os.Remove(tmpPath)
		return fmt.Errorf("file too small (%d bytes), likely error page", written)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp: %w", err)
	}

	return nil
}

// truncateURL 截断 URL 用于日志显示
func truncateURL(url string) string {
	if len(url) > 60 {
		return url[:57] + "..."
	}
	return url
}
