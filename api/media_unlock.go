package api

import (
	"clashgo/internal/unlock"
)

// MediaUnlockAPI 媒体解锁检测 API
type MediaUnlockAPI struct{}

func NewMediaUnlockAPI() *MediaUnlockAPI {
	return &MediaUnlockAPI{}
}

// GetUnlockItems 返回默认 Pending 占位列表
// 对应原: get_unlock_items
func (a *MediaUnlockAPI) GetUnlockItems() []unlock.UnlockItem {
	return unlock.DefaultUnlockItems()
}

// CheckMediaUnlock 并发检测所有平台解锁状态，返回真实结果
// 对应原: check_media_unlock
// 所有请求通过当前代理出口节点发出（无需额外配置）
func (a *MediaUnlockAPI) CheckMediaUnlock() ([]unlock.UnlockItem, error) {
	results := unlock.CheckAllMedia()
	return results, nil
}
