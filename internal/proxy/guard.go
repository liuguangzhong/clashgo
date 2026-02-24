// Package proxy - 代理守卫（对应原 src-tauri/src/core/sysopt.rs ProxyGuard）
// 功能：定期检查系统代理设置是否被第三方软件篡改，自动恢复

package proxy

import (
	"context"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"go.uber.org/zap"
)

const defaultGuardIntervalSec = 30

// Guard 代理守卫
type Guard struct {
	proxy  SysProxy
	cfgMgr *config.Manager
	cancel context.CancelFunc
}

// NewGuard 创建代理守卫
func NewGuard(sp SysProxy, cfgMgr *config.Manager) *Guard {
	return &Guard{proxy: sp, cfgMgr: cfgMgr}
}

// Start 启动守卫协程（根据配置决定是否运行）
func (g *Guard) Start(ctx context.Context) {
	verge := g.cfgMgr.GetVerge()
	if verge.EnableProxyGuard == nil || !*verge.EnableProxyGuard {
		return
	}

	ctx, g.cancel = context.WithCancel(ctx)

	interval := defaultGuardIntervalSec
	if verge.ProxyGuardDuration != nil && *verge.ProxyGuardDuration > 0 {
		interval = int(*verge.ProxyGuardDuration)
	}

	utils.Log().Info("Proxy guard started", zap.Int("interval_sec", interval))

	go func() {
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				utils.Log().Info("Proxy guard stopped")
				return
			case <-ticker.C:
				g.check()
			}
		}
	}()
}

// Stop 停止守卫
func (g *Guard) Stop() {
	if g.cancel != nil {
		g.cancel()
	}
}

// Refresh 在配置变更后立即执行一次检查（对应原 ProxyGuard::trigger()）
func (g *Guard) Refresh() {
	g.check()
}

// check 检查并恢复系统代理设置
func (g *Guard) check() {
	verge := g.cfgMgr.GetVerge()

	// 只在系统代理启用时才守卫
	if verge.EnableSystemProxy == nil || !*verge.EnableSystemProxy {
		return
	}

	// 检查当前系统代理是否与预期一致
	current, err := g.proxy.GetCurrentProxy()
	if err != nil {
		utils.Log().Warn("Guard: failed to get current proxy", zap.Error(err))
		return
	}

	host := "127.0.0.1"
	if verge.ProxyHost != nil {
		host = *verge.ProxyHost
	}
	expectedPort := 7897
	if verge.VergeMixedPort != nil {
		expectedPort = int(*verge.VergeMixedPort)
	}

	// 如果代理设置不匹配，自动恢复
	if !current.Enabled || current.Host != host || current.Port != expectedPort {
		utils.Log().Warn("Guard: proxy settings changed, restoring...",
			zap.Bool("enabled", current.Enabled),
			zap.String("actual_host", current.Host),
			zap.Int("actual_port", current.Port),
			zap.String("expected_host", host),
			zap.Int("expected_port", expectedPort),
		)

		if err := g.proxy.Apply(verge); err != nil {
			utils.Log().Error("Guard: failed to restore proxy", zap.Error(err))
		} else {
			utils.Log().Info("Guard: proxy settings restored")
		}
	}
}
