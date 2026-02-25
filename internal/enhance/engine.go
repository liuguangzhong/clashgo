package enhance

import (
	"fmt"
	"os"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Result 增强流水线输出
type Result struct {
	Config     map[string]interface{}
	ExistsKeys map[string]bool
	ChainLogs  map[string][][2]string // uid → [(level, msg)]
	Runtime    *config.RuntimeConfig
}

// Engine 配置增强流水线
// 对应原 src-tauri/src/enhance/mod.rs 的 enhance() 函数
type Engine struct {
	verge    config.IVerge
	clash    config.IClashBase
	profiles config.IProfiles
	dirs     *utils.AppDirs
}

// NewEngine 创建增强引擎实例
func NewEngine(
	verge config.IVerge,
	clash config.IClashBase,
	profiles config.IProfiles,
	dirs *utils.AppDirs,
) *Engine {
	return &Engine{
		verge:    verge,
		clash:    clash,
		profiles: profiles,
		dirs:     dirs,
	}
}

// Run 执行完整增强流水线，返回增强后的配置
// 流水线顺序（对应原 Rust 实现）:
//  1. 读取当前激活的订阅配置
//  2. Global Merge
//  3. Global Script (JS)
//  4. Profile Rules/Proxies/Groups 注入
//  5. Profile Merge
//  6. Profile Script (JS)
//  7. 合并 Clash 基础配置
//  8. 内置脚本（builtin scripts）
//  9. 清理无效代理组引用
//  10. TUN 配置注入
//  11. 字段排序
//  12. DNS 配置注入
func (e *Engine) Run() (*Result, error) {
	log := utils.Log()
	chainLogs := make(map[string][][2]string)
	existsKeys := make(map[string]bool)

	// ─── 步骤1：读取当前订阅的原始 YAML ───────────────────────────────────────
	baseConfig, profileName, err := e.loadCurrentProfile()
	if err != nil {
		log.Warn("[链路] No active profile, using empty config", zap.Error(err))
		baseConfig = make(map[string]interface{})
		profileName = ""
	} else {
		// 统计代理节点数
		proxyCount := 0
		if proxies, ok := baseConfig["proxies"].([]interface{}); ok {
			proxyCount = len(proxies)
		}
		groupCount := 0
		if groups, ok := baseConfig["proxy-groups"].([]interface{}); ok {
			groupCount = len(groups)
		}
		log.Info("[链路] 订阅配置已加载",
			zap.String("profile", profileName),
			zap.Int("keys", len(baseConfig)),
			zap.Int("proxies", proxyCount),
			zap.Int("proxy-groups", groupCount))
	}

	// 记录原始字段
	for k := range baseConfig {
		existsKeys[k] = true
	}

	// ─── 步骤2：Global Merge ──────────────────────────────────────────────────
	globalMergeItem := e.getChainItem("Merge")
	if globalMergeItem != nil && globalMergeItem.Type == ChainTypeMerge {
		baseConfig = ApplyMerge(globalMergeItem.Data.(map[string]interface{}), baseConfig)
		for k := range baseConfig {
			existsKeys[k] = true
		}
	}

	// ─── 步骤3：Global Script ─────────────────────────────────────────────────
	globalScriptItem := e.getChainItem("Script")
	if globalScriptItem != nil && globalScriptItem.Type == ChainTypeScript {
		result, logs, err := ExecuteScript(
			globalScriptItem.Data.(string),
			baseConfig,
			profileName,
		)
		chainLogs["Script"] = logs
		if err != nil {
			log.Warn("Global script error", zap.Error(err))
		} else {
			baseConfig = result
			for k := range baseConfig {
				existsKeys[k] = true
			}
		}
	}

	// ─── 步骤4-6：Profile 专属 chain（Rules/Proxies/Groups/Merge/Script）──────
	current := e.currentProfile()
	if current != nil {
		baseConfig, chainLogs = e.applyProfileChain(baseConfig, current, chainLogs, existsKeys, profileName)
	}

	// ─── 步骤7：合并 Clash 基础配置 ──────────────────────────────────────────
	socksEnabled := e.verge.VergeSocksEnabled != nil && *e.verge.VergeSocksEnabled
	httpEnabled := e.verge.VergeHttpEnabled != nil && *e.verge.VergeHttpEnabled
	redirEnabled := e.verge.VergeRedirEnabled != nil && *e.verge.VergeRedirEnabled
	tproxyEnabled := e.verge.VergeTProxyEnabled != nil && *e.verge.VergeTProxyEnabled
	externalCtrlEnabled := e.verge.EnableExternalController != nil && *e.verge.EnableExternalController

	baseConfig = MergeClashBase(baseConfig, e.clash, MergeClashBaseOpts{
		SocksEnabled:        socksEnabled,
		HTTPEnabled:         httpEnabled,
		RedirEnabled:        redirEnabled,
		TProxyEnabled:       tproxyEnabled,
		ExternalCtrlEnabled: externalCtrlEnabled,
	})

	// ─── 步骤7.5：注入 GeoIP/GeoSite 国内 CDN 下载地址 ────────────────────────
	// 默认从 GitHub 下载，国内无法访问。使用 jsdelivr CDN 作为 fallback。
	// 仅在用户未自行配置 geodata-url / geox-url 时注入。
	if _, hasGeox := baseConfig["geox-url"]; !hasGeox {
		baseConfig["geox-url"] = map[string]interface{}{
			"geoip":   "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb",
			"geosite": "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
			"mmdb":    "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
			"asn":     "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/GeoLite2-ASN.mmdb",
		}
	}

	// ─── 步骤8：内置脚本 ──────────────────────────────────────────────────────
	if e.verge.EnableBuiltinEnhanced == nil || *e.verge.EnableBuiltinEnhanced {
		coreName := e.verge.GetClashCore()
		baseConfig = ApplyBuiltinScripts(baseConfig, coreName)
	}

	// ─── 步骤9：清理无效代理组引用 ───────────────────────────────────────────
	baseConfig = CleanupProxyGroups(baseConfig)

	// ─── 步骤10：TUN 配置注入 ─────────────────────────────────────────────────
	tunEnabled := e.verge.EnableTunMode != nil && *e.verge.EnableTunMode
	baseConfig = ApplyTUN(baseConfig, tunEnabled)

	// ─── 步骤11：字段排序 ─────────────────────────────────────────────────────
	baseConfig = SortFields(baseConfig)

	// ─── 步骤12：DNS 配置注入 ─────────────────────────────────────────────────
	if e.verge.EnableDNSSettings != nil && *e.verge.EnableDNSSettings {
		baseConfig = e.applyDNSConfig(baseConfig)
	}

	runtime := config.NewRuntimeConfig()
	runtime.Config = baseConfig
	runtime.ExistsKeys = existsKeys
	runtime.ChainLogs = chainLogs

	return &Result{
		Config:     baseConfig,
		ExistsKeys: existsKeys,
		ChainLogs:  chainLogs,
		Runtime:    runtime,
	}, nil
}

// ─── 内部辅助方法 ─────────────────────────────────────────────────────────────

// loadCurrentProfile 读取当前激活订阅的原始 YAML（解析为 map）
func (e *Engine) loadCurrentProfile() (map[string]interface{}, string, error) {
	current := e.currentProfile()
	if current == nil {
		return nil, "", fmt.Errorf("no active profile")
	}

	profileName := ""
	if current.Name != nil {
		profileName = *current.Name
	}

	if current.File == nil {
		return make(map[string]interface{}), profileName, nil
	}

	filePath := e.dirs.ProfileFile(*current.File)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, profileName, fmt.Errorf("read profile file %s: %w", filePath, err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, profileName, fmt.Errorf("parse profile yaml: %w", err)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	return cfg, profileName, nil
}

// currentProfile 返回当前激活的 Profile 对象
func (e *Engine) currentProfile() *config.IProfile {
	if e.profiles.Current == nil {
		return nil
	}
	for _, p := range e.profiles.Items {
		if p.UID != nil && *p.UID == *e.profiles.Current {
			return p
		}
	}
	return nil
}

// getChainItem 按 uid 找到全局 chain 配置项并加载其内容
func (e *Engine) getChainItem(uid string) *ChainItem {
	for _, p := range e.profiles.Items {
		if p.UID != nil && *p.UID == uid {
			item, err := loadChainItem(p, e.dirs)
			if err != nil {
				utils.Log().Warn("Failed to load chain item",
					zap.String("uid", uid), zap.Error(err))
				return nil
			}
			return item
		}
	}
	return nil
}

// applyProfileChain 按顺序执行 profile 专属的 chain 处理
func (e *Engine) applyProfileChain(
	cfg map[string]interface{},
	profile *config.IProfile,
	chainLogs map[string][][2]string,
	existsKeys map[string]bool,
	profileName string,
) (map[string]interface{}, map[string][][2]string) {
	log := utils.Log()

	// Rules 注入
	if profile.CurrentRules != nil {
		if item := e.getChainItem(*profile.CurrentRules); item != nil && item.Type == ChainTypeRules {
			cfg = InjectSeq(cfg, item.Data.(SeqMap), "rules")
		}
	}

	// Proxies 注入
	if profile.CurrentProxies != nil {
		if item := e.getChainItem(*profile.CurrentProxies); item != nil && item.Type == ChainTypeProxies {
			cfg = InjectSeq(cfg, item.Data.(SeqMap), "proxies")
		}
	}

	// Groups 注入
	if profile.CurrentGroups != nil {
		if item := e.getChainItem(*profile.CurrentGroups); item != nil && item.Type == ChainTypeGroups {
			cfg = InjectSeq(cfg, item.Data.(SeqMap), "proxy-groups")
		}
	}

	// Profile Merge
	mergeUID := "Merge"
	if profile.CurrentMerge != nil {
		mergeUID = *profile.CurrentMerge
	}
	if item := e.getChainItem(mergeUID); item != nil && item.Type == ChainTypeMerge {
		cfg = ApplyMerge(item.Data.(map[string]interface{}), cfg)
		for k := range cfg {
			existsKeys[k] = true
		}
	}

	// Profile Script
	scriptUID := "Script"
	if profile.CurrentScript != nil {
		scriptUID = *profile.CurrentScript
	}
	if item := e.getChainItem(scriptUID); item != nil && item.Type == ChainTypeScript {
		result, logs, err := ExecuteScript(item.Data.(string), cfg, profileName)
		chainLogs[scriptUID] = logs
		if err != nil {
			log.Warn("Profile script error", zap.String("uid", scriptUID), zap.Error(err))
		} else {
			cfg = result
			for k := range cfg {
				existsKeys[k] = true
			}
		}
	}

	return cfg, chainLogs
}

// applyDNSConfig 读取 dns_config.yaml 并注入到配置中
func (e *Engine) applyDNSConfig(cfg map[string]interface{}) map[string]interface{} {
	dnsPath := e.dirs.DNSConfigPath()
	data, err := os.ReadFile(dnsPath)
	if err != nil {
		return cfg
	}

	var dnsCfg map[string]interface{}
	if err := yaml.Unmarshal(data, &dnsCfg); err != nil {
		return cfg
	}

	if hosts, ok := dnsCfg["hosts"]; ok {
		cfg["hosts"] = hosts
	}
	if dns, ok := dnsCfg["dns"]; ok {
		cfg["dns"] = dns
	}

	return cfg
}
