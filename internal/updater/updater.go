// Package updater 自动更新检查（对应原 tauri-plugin-updater 功能）
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"go.uber.org/zap"
)

const (
	githubReleasesURL = "https://api.github.com/repos/clash-verge-rev/clashgo/releases/latest"
	checkInterval     = 3 * time.Hour
)

// Updater 自动更新管理器
type Updater struct {
	ctx    context.Context
	cfgMgr *config.Manager
	cancel context.CancelFunc

	// 通知回调（Wails EventsEmit 由 app.go 注入）
	onUpdateAvailable func(version, notes string)
}

// NewUpdater 创建更新管理器
func NewUpdater(ctx context.Context, cfg *config.Manager) *Updater {
	ctx, cancel := context.WithCancel(ctx)
	return &Updater{ctx: ctx, cfgMgr: cfg, cancel: cancel}
}

// SetUpdateCallback 注入更新可用时的通知回调
func (u *Updater) SetUpdateCallback(fn func(version, notes string)) {
	u.onUpdateAvailable = fn
}

// StartAutoCheck 启动定期检查（每3小时）
func (u *Updater) StartAutoCheck() {
	verge := u.cfgMgr.GetVerge()
	if verge.AutoCheckUpdate == nil || !*verge.AutoCheckUpdate {
		return
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		// 启动约30秒后执行首次检查（避免与启动争抢资源）
		select {
		case <-u.ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}
		u.CheckUpdate()

		for {
			select {
			case <-u.ctx.Done():
				return
			case <-ticker.C:
				u.CheckUpdate()
			}
		}
	}()
}

// CheckUpdate 向 GitHub Releases API 检查是否有新版本
func (u *Updater) CheckUpdate() {
	log := utils.Log()
	log.Info("Checking for updates...")

	latest, err := fetchLatestRelease()
	if err != nil {
		log.Warn("Update check failed", zap.Error(err))
		return
	}

	current := config.EmbeddedMihomoVersion
	if isNewerVersion(latest.TagName, current) {
		log.Info("Update available",
			zap.String("current", current),
			zap.String("latest", latest.TagName),
		)
		if u.onUpdateAvailable != nil {
			u.onUpdateAvailable(latest.TagName, latest.Body)
		}
	} else {
		log.Info("Already up to date", zap.String("version", current))
	}
}

// Stop 停止自动更新
func (u *Updater) Stop() {
	if u.cancel != nil {
		u.cancel()
	}
	utils.Log().Info("Updater stopped")
}

// ─── GitHub API ───────────────────────────────────────────────────────────────

// githubRelease GitHub Release 响应结构
type githubRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"` // release notes
}

// fetchLatestRelease 请求 GitHub API 获取最新版本信息
func fetchLatestRelease() (*githubRelease, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ClashGo/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &release, nil
}

// isNewerVersion 比较版本号字符串（简单的字符串比较适合语义化版本）
// latest="v1.2.0", current="v1.1.5" → true
func isNewerVersion(latest, current string) bool {
	// 规范化：去掉 v 前缀
	l := strings.TrimPrefix(latest, "v")
	c := strings.TrimPrefix(current, "v")

	if l == c {
		return false
	}

	// 简单分段比较：1.2.0 vs 1.1.5
	lParts := strings.Split(l, ".")
	cParts := strings.Split(c, ".")

	maxLen := len(lParts)
	if len(cParts) > maxLen {
		maxLen = len(cParts)
	}

	for i := 0; i < maxLen; i++ {
		lv, cv := 0, 0
		if i < len(lParts) {
			fmt.Sscanf(lParts[i], "%d", &lv)
		}
		if i < len(cParts) {
			fmt.Sscanf(cParts[i], "%d", &cv)
		}
		if lv > cv {
			return true
		}
		if lv < cv {
			return false
		}
	}
	return false
}
