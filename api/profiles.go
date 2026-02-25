package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/enhance"
	"clashgo/internal/utils"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ProfileAPI 订阅管理 API（对应原 cmd/profile 系列命令）
type ProfileAPI struct {
	mgr *config.Manager
}

func NewProfileAPI(mgr *config.Manager) *ProfileAPI {
	return &ProfileAPI{mgr: mgr}
}

// GetProfiles 获取所有订阅配置
func (a *ProfileAPI) GetProfiles() config.IProfiles {
	return a.mgr.GetProfiles()
}

// ImportProfile 通过 URL 导入订阅
func (a *ProfileAPI) ImportProfile(req ImportProfileRequest) error {
	if req.URL == "" {
		return fmt.Errorf("URL is required")
	}

	log := utils.Log()
	log.Info("Importing profile", zap.String("url", maskURL(req.URL)))

	content, extra, err := downloadProfile(req.URL, req.Option)
	if err != nil {
		return fmt.Errorf("download profile: %w", err)
	}

	uid := uuid.New().String()[:8]
	filename := uid + ".yaml"

	filePath := utils.Dirs().ProfileFile(filename)
	if err := config.WriteFileAtomic(filePath, content); err != nil {
		return fmt.Errorf("write profile file: %w", err)
	}

	profileName := req.Name
	if profileName == "" {
		profileName = extractNameFromURL(req.URL)
	}

	profileType := "remote"
	now := time.Now()
	profile := &config.IProfile{
		UID:       &uid,
		Type:      &profileType,
		Name:      &profileName,
		URL:       &req.URL,
		File:      &filename,
		Extra:     extra,
		UpdatedAt: &now,
		Option:    req.Option,
	}

	return a.mgr.AddProfile(profile)
}

// CreateProfile 创建本地配置文件（非订阅）
func (a *ProfileAPI) CreateProfile(req CreateProfileRequest) error {
	uid := uuid.New().String()[:8]
	filename := uid + ".yaml"

	profileType := req.Type
	if profileType == "" {
		profileType = "local"
	}

	filePath := utils.Dirs().ProfileFile(filename)
	initContent := req.Content
	if initContent == "" {
		initContent = "# ClashGo Profile\n"
	}
	if err := config.WriteFileAtomic(filePath, []byte(initContent)); err != nil {
		return fmt.Errorf("write profile: %w", err)
	}

	now := time.Now()
	profile := &config.IProfile{
		UID:       &uid,
		Type:      &profileType,
		Name:      &req.Name,
		File:      &filename,
		UpdatedAt: &now,
		Option:    req.Option,
	}
	if req.Desc != "" {
		profile.Desc = &req.Desc
	}
	if req.URL != "" {
		profile.URL = &req.URL
	}

	return a.mgr.AddProfile(profile)
}

// PatchProfile 修改订阅配置信息（名称、更新间隔等）
func (a *ProfileAPI) PatchProfile(uid string, patch config.IProfile) error {
	return a.mgr.UpdateProfile(uid, &patch)
}

// DeleteProfile 删除订阅
func (a *ProfileAPI) DeleteProfile(uid string) error {
	return a.mgr.DeleteProfile(uid)
}

// EnhanceProfiles 触发增强流水线（切换配置或修改 chain 后调用）
func (a *ProfileAPI) EnhanceProfiles() error {
	utils.Log().Info("[链路] EnhanceProfiles → UpdateConfig")
	if coreManagerRef == nil {
		utils.Log().Error("[链路] coreManagerRef is nil!")
		return fmt.Errorf("core manager not initialized")
	}
	err := coreManagerRef.UpdateConfig()
	if err != nil {
		utils.Log().Error("[链路] UpdateConfig 失败", zap.Error(err))
	} else {
		utils.Log().Info("[链路] UpdateConfig 成功")
	}
	return err
}

// PatchProfilesConfig 将指定 profile 设为当前配置
func (a *ProfileAPI) PatchProfilesConfig(uid string) error {
	utils.Log().Info("[链路] PatchProfilesConfig 开始", zap.String("uid", uid))
	if err := a.mgr.SetCurrentProfile(uid); err != nil {
		utils.Log().Error("[链路] SetCurrentProfile 失败", zap.Error(err))
		return err
	}
	utils.Log().Info("[链路] SetCurrentProfile 成功，开始 EnhanceProfiles")
	if err := a.EnhanceProfiles(); err != nil {
		utils.Log().Error("[链路] EnhanceProfiles 失败", zap.Error(err))
		return err
	}
	utils.Log().Info("[链路] PatchProfilesConfig 完成")
	return nil
}

// UpdateProfile 从网络更新远程订阅
// option 可为 nil，表示使用 profile 自身保存的 option
func (a *ProfileAPI) UpdateProfile(uid string, option *config.ProfileOption) error {
	profiles := a.mgr.GetProfiles()

	var target *config.IProfile
	for _, p := range profiles.Items {
		if p.UID != nil && *p.UID == uid {
			target = p
			break
		}
	}
	if target == nil {
		return fmt.Errorf("profile not found: %s", uid)
	}
	if target.URL == nil || *target.URL == "" {
		return fmt.Errorf("profile has no URL (local-only)")
	}

	// 使用前端传入的 option 覆盖，否则用 profile 自身的
	downloadOpt := target.Option
	if option != nil {
		downloadOpt = option
	}

	content, extra, err := downloadProfile(*target.URL, downloadOpt)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}

	if target.File == nil {
		return fmt.Errorf("profile has no file path")
	}

	filePath := utils.Dirs().ProfileFile(*target.File)
	if err := config.WriteFileAtomic(filePath, content); err != nil {
		return fmt.Errorf("write updated profile: %w", err)
	}

	now := time.Now()
	patch := &config.IProfile{Extra: extra, UpdatedAt: &now}
	return a.mgr.UpdateProfile(uid, patch)
}

// ReadProfileFile 读取 Profile 文件内容
func (a *ProfileAPI) ReadProfileFile(uid string) (string, error) {
	profiles := a.mgr.GetProfiles()
	for _, p := range profiles.Items {
		if p.UID != nil && *p.UID == uid && p.File != nil {
			data, err := os.ReadFile(utils.Dirs().ProfileFile(*p.File))
			if err != nil {
				return "", err
			}
			return string(data), nil
		}
	}
	return "", fmt.Errorf("profile not found: %s", uid)
}

// SaveProfileFile 保存 Profile 文件内容（编辑器直接写）
func (a *ProfileAPI) SaveProfileFile(uid string, content string) error {
	profiles := a.mgr.GetProfiles()
	for _, p := range profiles.Items {
		if p.UID != nil && *p.UID == uid && p.File != nil {
			return config.WriteFileAtomic(utils.Dirs().ProfileFile(*p.File), []byte(content))
		}
	}
	return fmt.Errorf("profile not found: %s", uid)
}

// ValidateScriptFile 验证 JS 脚本语法（通过 goja 实际解析）
func (a *ProfileAPI) ValidateScriptFile(uid string) error {
	content, err := a.ReadProfileFile(uid)
	if err != nil {
		return err
	}
	return enhance.ValidateScript(content)
}

// ReorderProfile 调整订阅顺序（拖拽排序）
// activeUID 是被拖动的项，overUID 是放置目标位置的项
func (a *ProfileAPI) ReorderProfile(activeUID, overUID string) error {
	profiles := a.mgr.GetProfiles()
	items := profiles.Items

	activeIdx, overIdx := -1, -1
	for i, p := range items {
		if p.UID != nil {
			if *p.UID == activeUID {
				activeIdx = i
			}
			if *p.UID == overUID {
				overIdx = i
			}
		}
	}
	if activeIdx < 0 || overIdx < 0 {
		return fmt.Errorf("profile not found for reorder (active=%s, over=%s)", activeUID, overUID)
	}

	// 从原位移除
	item := items[activeIdx]
	without := make([]*config.IProfile, 0, len(items)-1)
	for i, p := range items {
		if i != activeIdx {
			without = append(without, p)
		}
	}

	// 计算插入位置（over 位置在移除 active 后可能偏移）
	insertAt := overIdx
	if activeIdx < overIdx {
		insertAt--
	}
	// 插入到 insertAt 之后
	insertAt++
	if insertAt > len(without) {
		insertAt = len(without)
	}

	result := make([]*config.IProfile, 0, len(items))
	result = append(result, without[:insertAt]...)
	result = append(result, item)
	result = append(result, without[insertAt:]...)

	// 通过 Manager 保存重排后的列表
	return a.mgr.ReorderProfiles(result)
}

// ─── 请求类型 ─────────────────────────────────────────────────────────────────

// ImportProfileRequest 导入请求
type ImportProfileRequest struct {
	URL    string                `json:"url"`
	Name   string                `json:"name,omitempty"`
	Option *config.ProfileOption `json:"option,omitempty"`
}

// CreateProfileRequest 创建本地配置请求
type CreateProfileRequest struct {
	Type    string                `json:"type"`
	Name    string                `json:"name"`
	Desc    string                `json:"desc,omitempty"`
	URL     string                `json:"url,omitempty"`
	Content string                `json:"content,omitempty"`
	Option  *config.ProfileOption `json:"option,omitempty"`
}

// ─── 内部辅助 ─────────────────────────────────────────────────────────────────

// downloadProfile 下载订阅内容并解析元数据
func downloadProfile(rawURL string, opt *config.ProfileOption) ([]byte, *config.ProfileExtra, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, nil, err
	}

	ua := "ClashVerge/1.0 (clashgo)"
	if opt != nil && opt.UserAgent != nil {
		ua = *opt.UserAgent
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read body: %w", err)
	}

	// 验证 YAML 合法性
	var check map[string]interface{}
	if err := yaml.Unmarshal(body, &check); err != nil {
		return nil, nil, fmt.Errorf("not valid YAML: %w", err)
	}

	// 解析 subscription-userinfo 响应头
	extra := parseSubscriptionInfo(resp.Header.Get("subscription-userinfo"))

	return body, extra, nil
}

// parseSubscriptionInfo 解析 subscription-userinfo 响应头
// 格式(顺序不定): upload=100; download=200; total=10000000000; expire=1893456000
func parseSubscriptionInfo(header string) *config.ProfileExtra {
	if header == "" {
		return nil
	}
	extra := &config.ProfileExtra{}
	hasData := false
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])
		var n int64
		if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
			continue
		}
		hasData = true
		switch key {
		case "upload":
			extra.Upload = n
		case "download":
			extra.Download = n
		case "total":
			extra.Total = n
		case "expire":
			extra.Expire = n
		}
	}
	if !hasData {
		return nil
	}
	return extra
}

// extractNameFromURL 从 URL 提取订阅名称
func extractNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "Subscription"
	}
	base := filepath.Base(parsed.Path)
	if base == "" || base == "/" || base == "." {
		return parsed.Host
	}
	return base
}

// maskURL 对 URL 中敏感部分打码（日志安全）
func maskURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "***"
	}
	parsed.RawQuery = "" // 移除 query 参数（通常含 token）
	return parsed.String()
}

// coreManagerRef 由 app.go 在初始化时设置（UpdateConfig 接口）
var coreManagerRef interface {
	UpdateConfig() error
}

// coreRestarter 扩展接口：包含 Restart 方法
var coreRestarter interface {
	Restart() error
}

// SetCoreManager 供 app.go 在初始化后注入 core.Manager
// core.Manager 同时满足 coreManagerRef 和 coreRestarter 接口
func SetCoreManager(cm interface {
	UpdateConfig() error
	Restart() error
}) {
	coreManagerRef = cm
	coreRestarter = cm
}
