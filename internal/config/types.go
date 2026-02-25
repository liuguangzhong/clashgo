package config

import (
	"sync"
	"time"
)

// IVerge 对应原 src-tauri/src/config/verge.rs 的 IVerge 结构体
// 存储在 verge.yaml，控制 GUI 行为和代理设置
type IVerge struct {
	// 日志
	AppLogLevel    *string `yaml:"app_log_level,omitempty" json:"app_log_level,omitempty"`
	AppLogMaxSize  *uint64 `yaml:"app_log_max_size,omitempty" json:"app_log_max_size,omitempty"`
	AppLogMaxCount *int    `yaml:"app_log_max_count,omitempty" json:"app_log_max_count,omitempty"`

	// 界面
	Language       *string `yaml:"language,omitempty" json:"language,omitempty"`
	ThemeMode      *string `yaml:"theme_mode,omitempty" json:"theme_mode,omitempty"`
	CollapseNavbar *bool   `yaml:"collapse_navbar,omitempty" json:"collapse_navbar,omitempty"`
	StartPage      *string `yaml:"start_page,omitempty" json:"start_page,omitempty"`
	NoticePosition *string `yaml:"notice_position,omitempty" json:"notice_position,omitempty"`
	MenuIcon       *string `yaml:"menu_icon,omitempty" json:"menu_icon,omitempty"`

	// 代理设置
	EnableSystemProxy  *bool   `yaml:"enable_system_proxy,omitempty" json:"enable_system_proxy,omitempty"`
	ProxyAutoConfig    *bool   `yaml:"proxy_auto_config,omitempty" json:"proxy_auto_config,omitempty"`
	PacFileContent     *string `yaml:"pac_file_content,omitempty" json:"pac_file_content,omitempty"`
	ProxyHost          *string `yaml:"proxy_host,omitempty" json:"proxy_host,omitempty"`
	SystemProxyBypass  *string `yaml:"system_proxy_bypass,omitempty" json:"system_proxy_bypass,omitempty"`
	UseDefaultBypass   *bool   `yaml:"use_default_bypass,omitempty" json:"use_default_bypass,omitempty"`
	EnableProxyGuard   *bool   `yaml:"enable_proxy_guard,omitempty" json:"enable_proxy_guard,omitempty"`
	ProxyGuardDuration *uint64 `yaml:"proxy_guard_duration,omitempty" json:"proxy_guard_duration,omitempty"`
	EnableBypassCheck  *bool   `yaml:"enable_bypass_check,omitempty" json:"enable_bypass_check,omitempty"`

	// TUN 模式
	EnableTunMode *bool `yaml:"enable_tun_mode,omitempty" json:"enable_tun_mode,omitempty"`

	// 端口（Linux/macOS 额外路由端口）
	VergeMixedPort    *uint16 `yaml:"verge_mixed_port,omitempty" json:"verge_mixed_port,omitempty"`
	VergeSocksPort    *uint16 `yaml:"verge_socks_port,omitempty" json:"verge_socks_port,omitempty"`
	VergeSocksEnabled *bool   `yaml:"verge_socks_enabled,omitempty" json:"verge_socks_enabled,omitempty"`
	VergePort         *uint16 `yaml:"verge_port,omitempty" json:"verge_port,omitempty"`
	VergeHttpEnabled  *bool   `yaml:"verge_http_enabled,omitempty" json:"verge_http_enabled,omitempty"`
	// Linux/macOS 专属
	VergeRedirPort    *uint16 `yaml:"verge_redir_port,omitempty" json:"verge_redir_port,omitempty"`
	VergeRedirEnabled *bool   `yaml:"verge_redir_enabled,omitempty" json:"verge_redir_enabled,omitempty"`
	// Linux 专属
	VergeTProxyPort    *uint16 `yaml:"verge_tproxy_port,omitempty" json:"verge_tproxy_port,omitempty"`
	VergeTProxyEnabled *bool   `yaml:"verge_tproxy_enabled,omitempty" json:"verge_tproxy_enabled,omitempty"`

	// 启动控制
	EnableAutoLaunch  *bool   `yaml:"enable_auto_launch,omitempty" json:"enable_auto_launch,omitempty"`
	EnableSilentStart *bool   `yaml:"enable_silent_start,omitempty" json:"enable_silent_start,omitempty"`
	StartupScript     *string `yaml:"startup_script,omitempty" json:"startup_script,omitempty"`

	// 代理核心
	ClashCore *string `yaml:"clash_core,omitempty" json:"clash_core,omitempty"`

	// 热键
	Hotkeys            *[]string `yaml:"hotkeys,omitempty" json:"hotkeys,omitempty"`
	EnableGlobalHotkey *bool     `yaml:"enable_global_hotkey,omitempty" json:"enable_global_hotkey,omitempty"`

	// 连接行为
	AutoCloseConnection *bool `yaml:"auto_close_connection,omitempty" json:"auto_close_connection,omitempty"`

	// 更新
	AutoCheckUpdate *bool `yaml:"auto_check_update,omitempty" json:"auto_check_update,omitempty"`

	// 延迟测试
	DefaultLatencyTest    *string `yaml:"default_latency_test,omitempty" json:"default_latency_test,omitempty"`
	DefaultLatencyTimeout *int16  `yaml:"default_latency_timeout,omitempty" json:"default_latency_timeout,omitempty"`

	// 增强功能
	EnableBuiltinEnhanced *bool `yaml:"enable_builtin_enhanced,omitempty" json:"enable_builtin_enhanced,omitempty"`

	// 日志清理 (0:不清理 1:1天 2:7天 3:30天 4:90天)
	AutoLogClean *int32 `yaml:"auto_log_clean,omitempty" json:"auto_log_clean,omitempty"`

	// 备份
	EnableAutoBackupSchedule *bool   `yaml:"enable_auto_backup_schedule,omitempty" json:"enable_auto_backup_schedule,omitempty"`
	AutoBackupIntervalHours  *uint64 `yaml:"auto_backup_interval_hours,omitempty" json:"auto_backup_interval_hours,omitempty"`
	AutoBackupOnChange       *bool   `yaml:"auto_backup_on_change,omitempty" json:"auto_backup_on_change,omitempty"`

	// WebDAV（加密存储）
	WebDAVUrl      *string `yaml:"webdav_url,omitempty" json:"webdav_url,omitempty"`
	WebDAVUsername *string `yaml:"webdav_username,omitempty" json:"webdav_username,omitempty"`
	WebDAVPassword *string `yaml:"webdav_password,omitempty" json:"webdav_password,omitempty"`

	// 托盘
	SysproxyTrayIcon        *bool   `yaml:"sysproxy_tray_icon,omitempty" json:"sysproxy_tray_icon,omitempty"`
	TunTrayIcon             *bool   `yaml:"tun_tray_icon,omitempty" json:"tun_tray_icon,omitempty"`
	TrayProxyGroupsDisplay  *string `yaml:"tray_proxy_groups_display_mode,omitempty" json:"tray_proxy_groups_display_mode,omitempty"`
	TrayInlineOutboundModes *bool   `yaml:"tray_inline_outbound_modes,omitempty" json:"tray_inline_outbound_modes,omitempty"`

	// 轻量模式
	EnableAutoLightWeightMode *bool   `yaml:"enable_auto_light_weight_mode,omitempty" json:"enable_auto_light_weight_mode,omitempty"`
	AutoLightWeightMinutes    *uint64 `yaml:"auto_light_weight_minutes,omitempty" json:"auto_light_weight_minutes,omitempty"`

	// DNS 设置
	EnableDNSSettings *bool `yaml:"enable_dns_settings,omitempty" json:"enable_dns_settings,omitempty"`

	// 外部控制器
	EnableExternalController *bool `yaml:"enable_external_controller,omitempty" json:"enable_external_controller,omitempty"`

	// 主题
	ThemeSetting *IVergeTheme `yaml:"theme_setting,omitempty" json:"theme_setting,omitempty"`

	// 布局
	ProxyLayoutColumn *uint8 `yaml:"proxy_layout_column,omitempty" json:"proxy_layout_column,omitempty"`

	// 首页卡片
	HomeCards interface{} `yaml:"home_cards,omitempty" json:"home_cards,omitempty"`
}

// IVergeTheme 主题配置
type IVergeTheme struct {
	PrimaryColor   *string `yaml:"primary_color,omitempty" json:"primary_color,omitempty"`
	SecondaryColor *string `yaml:"secondary_color,omitempty" json:"secondary_color,omitempty"`
	PrimaryText    *string `yaml:"primary_text,omitempty" json:"primary_text,omitempty"`
	SecondaryText  *string `yaml:"secondary_text,omitempty" json:"secondary_text,omitempty"`
	InfoColor      *string `yaml:"info_color,omitempty" json:"info_color,omitempty"`
	ErrorColor     *string `yaml:"error_color,omitempty" json:"error_color,omitempty"`
	WarningColor   *string `yaml:"warning_color,omitempty" json:"warning_color,omitempty"`
	SuccessColor   *string `yaml:"success_color,omitempty" json:"success_color,omitempty"`
	FontFamily     *string `yaml:"font_family,omitempty" json:"font_family,omitempty"`
	CSSInjection   *string `yaml:"css_injection,omitempty" json:"css_injection,omitempty"`
}

// ValidClashCores 有效的核心名称
var ValidClashCores = []string{"verge-mihomo", "verge-mihomo-alpha"}

// GetClashCore 返回有效的核心名称，默认 verge-mihomo
func (v *IVerge) GetClashCore() string {
	if v.ClashCore != nil {
		for _, valid := range ValidClashCores {
			if *v.ClashCore == valid {
				return *v.ClashCore
			}
		}
	}
	return "verge-mihomo"
}

// DefaultVerge 返回默认配置（对应原 IVerge::template()）
func DefaultVerge() IVerge {
	boolTrue := true
	boolFalse := false
	int32Two := int32(2)
	uint64_24 := uint64(24)
	uint64_30 := uint64(30)
	uint64_10 := uint64(10)
	uint16_17895 := uint16(17895)
	uint16_17896 := uint16(17896)
	uint16_17897 := uint16(17897)
	uint16_17898 := uint16(17898)
	uint16_17899 := uint16(17899)
	int8Timeout := int16(5000)
	themeSystem := "system"
	langEn := "en"
	startPage := "/"
	noticePos := "top-right"
	menuIcon := "monochrome"
	proxyHost := "127.0.0.1"
	defaultLatency := "https://www.gstatic.com/generate_204"
	displayMode := "default"
	coreDefault := "verge-mihomo"

	return IVerge{
		AppLogMaxSize:             ptrUint64(128),
		AppLogMaxCount:            ptrInt(8),
		ClashCore:                 &coreDefault,
		Language:                  &langEn,
		ThemeMode:                 &themeSystem,
		StartPage:                 &startPage,
		NoticePosition:            &noticePos,
		MenuIcon:                  &menuIcon,
		CollapseNavbar:            &boolFalse,
		EnableAutoLaunch:          &boolFalse,
		EnableSilentStart:         &boolFalse,
		EnableSystemProxy:         &boolFalse,
		ProxyAutoConfig:           &boolFalse,
		ProxyHost:                 &proxyHost,
		VergeRedirPort:            &uint16_17895,
		VergeRedirEnabled:         &boolFalse,
		VergeTProxyPort:           &uint16_17896,
		VergeTProxyEnabled:        &boolFalse,
		VergeMixedPort:            &uint16_17897,
		VergeSocksPort:            &uint16_17898,
		VergeSocksEnabled:         &boolFalse,
		VergePort:                 &uint16_17899,
		VergeHttpEnabled:          &boolFalse,
		EnableProxyGuard:          &boolFalse,
		EnableBypassCheck:         &boolTrue,
		UseDefaultBypass:          &boolTrue,
		ProxyGuardDuration:        &uint64_30,
		AutoCloseConnection:       &boolTrue,
		AutoCheckUpdate:           &boolTrue,
		DefaultLatencyTest:        &defaultLatency,
		DefaultLatencyTimeout:     &int8Timeout,
		EnableBuiltinEnhanced:     &boolTrue,
		AutoLogClean:              &int32Two,
		EnableAutoBackupSchedule:  &boolFalse,
		AutoBackupIntervalHours:   &uint64_24,
		AutoBackupOnChange:        &boolTrue,
		EnableGlobalHotkey:        &boolTrue,
		SysproxyTrayIcon:          &boolFalse,
		TunTrayIcon:               &boolFalse,
		TrayProxyGroupsDisplay:    &displayMode,
		TrayInlineOutboundModes:   &boolFalse,
		EnableAutoLightWeightMode: &boolFalse,
		AutoLightWeightMinutes:    &uint64_10,
		EnableDNSSettings:         &boolFalse,
		EnableExternalController:  &boolTrue,
	}
}

func ptrUint64(v uint64) *uint64 { return &v }
func ptrInt(v int) *int          { return &v }

// ─────────────────────────────────────────────────────────────────────────────
// IClashBase - 存储在 clash.yaml，控制 Mihomo 基础行为
// ─────────────────────────────────────────────────────────────────────────────

// IClashBase Clash 基础配置（YAML 键名与 Mihomo 一致）
type IClashBase map[string]interface{}

// ─────────────────────────────────────────────────────────────────────────────
// IProfiles - 订阅管理
// ─────────────────────────────────────────────────────────────────────────────

// IProfiles 存储在 profiles.yaml
type IProfiles struct {
	Current *string     `yaml:"current,omitempty" json:"current,omitempty"`
	Items   []*IProfile `yaml:"items,omitempty" json:"items,omitempty"`
}

// IProfile 单个订阅/配置文件
type IProfile struct {
	UID       *string        `yaml:"uid,omitempty" json:"uid,omitempty"`
	Type      *string        `yaml:"type,omitempty" json:"type,omitempty"` // "remote" | "local" | "merge" | "script"
	Name      *string        `yaml:"name,omitempty" json:"name,omitempty"`
	Desc      *string        `yaml:"desc,omitempty" json:"desc,omitempty"`
	File      *string        `yaml:"file,omitempty" json:"file,omitempty"`
	URL       *string        `yaml:"url,omitempty" json:"url,omitempty"`
	Selected  []Selected     `yaml:"selected,omitempty" json:"selected,omitempty"`
	Extra     *ProfileExtra  `yaml:"extra,omitempty" json:"extra,omitempty"`
	UpdatedAt *time.Time     `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
	Interval  *uint64        `yaml:"interval,omitempty" json:"interval,omitempty"` // 自动更新间隔（秒）
	Option    *ProfileOption `yaml:"option,omitempty" json:"option,omitempty"`
	// chain 配置
	CurrentMerge   *string `yaml:"current_merge,omitempty" json:"current_merge,omitempty"`
	CurrentScript  *string `yaml:"current_script,omitempty" json:"current_script,omitempty"`
	CurrentRules   *string `yaml:"current_rules,omitempty" json:"current_rules,omitempty"`
	CurrentProxies *string `yaml:"current_proxies,omitempty" json:"current_proxies,omitempty"`
	CurrentGroups  *string `yaml:"current_groups,omitempty" json:"current_groups,omitempty"`
}

// Selected 记录代理组选择
type Selected struct {
	Name string `yaml:"name" json:"name"`
	Now  string `yaml:"now" json:"now"`
}

// ProfileExtra 订阅元数据（从响应头解析）
type ProfileExtra struct {
	Upload   int64 `yaml:"upload" json:"upload"`
	Download int64 `yaml:"download" json:"download"`
	Total    int64 `yaml:"total" json:"total"`
	Expire   int64 `yaml:"expire" json:"expire"`
}

// ProfileOption 订阅下载选项
type ProfileOption struct {
	UserAgent      *string `yaml:"user_agent,omitempty" json:"user_agent,omitempty"`
	WithProxy      *bool   `yaml:"with_proxy,omitempty" json:"with_proxy,omitempty"`
	SelfProxy      *bool   `yaml:"self_proxy,omitempty" json:"self_proxy,omitempty"`
	UpdateInterval *uint64 `yaml:"update_interval,omitempty" json:"update_interval,omitempty"`
}

// ClashInfo 当前 Clash 连接信息（返回给前端）
type ClashInfo struct {
	MixedPort uint16  `json:"mixed_port"`
	SocksPort uint16  `json:"socks_port"`
	HTTPPort  uint16  `json:"port"`
	Server    string  `json:"server"`
	Secret    *string `json:"secret,omitempty"`
}

// RuntimeConfig 运行时配置（增强后的完整配置）
type RuntimeConfig struct {
	Config     IClashBase             `json:"config"`
	ExistsKeys map[string]bool        `json:"exists_keys"`
	ChainLogs  map[string][][2]string `json:"chain_logs"`
	mu         sync.RWMutex
}

func NewRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		Config:     make(IClashBase),
		ExistsKeys: make(map[string]bool),
		ChainLogs:  make(map[string][][2]string),
	}
}
