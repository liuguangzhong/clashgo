package api

import (
	"fmt"
	"os"

	"clashgo/internal/config"
	"clashgo/internal/utils"

	"gopkg.in/yaml.v3"
)

// ─── DNS 配置管理 ─────────────────────────────────────────────────────────────
// 对应原 clash.rs 中 save_dns_config / apply_dns_config / check_dns_config_exists
// / get_dns_config_content / validate_dns_config
//
// 架构：DNS 配置存储在独立文件 dns_config.yaml，
// 应用时合并到运行时配置触发 Mihomo 热加载。

// SaveDNSConfig 保存 DNS 配置到 dns_config.yaml
// 对应原: save_dns_config
func (a *ConfigAPI) SaveDNSConfig(dnsConfig map[string]interface{}) error {
	path := utils.Dirs().DNSConfigPath()

	data, err := yaml.Marshal(dnsConfig)
	if err != nil {
		return fmt.Errorf("marshal dns config: %w", err)
	}

	if err := config.WriteFileAtomic(path, data); err != nil {
		return fmt.Errorf("write dns config: %w", err)
	}

	utils.Log().Info("DNS config saved to " + path)
	return nil
}

// ApplyDNSConfig 应用或撤销 DNS 配置
// apply=true: 从 dns_config.yaml 读取并合并到 clash 基础配置层，触发热加载
// apply=false: 从 clash 基础配置层删除 dns 键（让 profile 自身的 DNS 生效），触发热加载
// 对应原: apply_dns_config
func (a *ConfigAPI) ApplyDNSConfig(apply bool) error {
	if apply {
		path := utils.Dirs().DNSConfigPath()
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("dns config file not found: %w", err)
		}

		var dnsMap map[string]interface{}
		if err := yaml.Unmarshal(data, &dnsMap); err != nil {
			return fmt.Errorf("parse dns config: %w", err)
		}

		// 将 dns 节点合并到 clash 基础配置
		patch := map[string]interface{}{"dns": dnsMap}
		if err := a.mgr.PatchClash(patch); err != nil {
			return fmt.Errorf("patch clash with dns: %w", err)
		}
	} else {
		// 撤销：从 clash 基础配置删除 dns 键，让 profile 自身的 DNS 配置生效
		// 注意：设为 nil 会在 YAML 里写 dns: null 覆盖 profile；
		// 正确做法是彻底删除该键
		if err := a.mgr.DeleteClashKey("dns"); err != nil {
			return fmt.Errorf("remove dns from clash: %w", err)
		}
	}

	// 触发核心热加载
	if coreManagerRef != nil {
		return coreManagerRef.UpdateConfig()
	}
	return nil
}

// CheckDNSConfigExists 检查 dns_config.yaml 是否存在
// 对应原: check_dns_config_exists
func (a *ConfigAPI) CheckDNSConfigExists() bool {
	_, err := os.Stat(utils.Dirs().DNSConfigPath())
	return err == nil
}

// GetDNSConfigContent 获取 dns_config.yaml 文本内容
// 对应原: get_dns_config_content
func (a *ConfigAPI) GetDNSConfigContent() (string, error) {
	path := utils.Dirs().DNSConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("dns config not found: %w", err)
	}
	return string(data), nil
}

// ValidateDNSConfig 验证 dns_config.yaml 语法是否合法
// 对应原: validate_dns_config
func (a *ConfigAPI) ValidateDNSConfig() (bool, string) {
	path := utils.Dirs().DNSConfigPath()
	if _, err := os.Stat(path); err != nil {
		return false, "DNS config file not found"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("Failed to read file: %v", err)
	}

	var check map[string]interface{}
	if err := yaml.Unmarshal(data, &check); err != nil {
		return false, fmt.Sprintf("YAML syntax error: %v", err)
	}

	return true, ""
}

// CopyClashEnv 生成代理环境变量文本（前端复制到剪贴板）
// 对应原: copy_clash_env
func (a *ConfigAPI) CopyClashEnv() string {
	clash := a.mgr.GetClash()
	mixedPort := 17897
	if p, ok := clash["mixed-port"].(int); ok && p > 0 {
		mixedPort = p
	}

	verge := a.mgr.GetVerge()
	if verge.VergeMixedPort != nil && *verge.VergeMixedPort > 0 {
		mixedPort = int(*verge.VergeMixedPort)
	}

	proxyAddr := fmt.Sprintf("http://127.0.0.1:%d", mixedPort)
	return fmt.Sprintf(
		"export https_proxy=%s\nexport http_proxy=%s\nexport all_proxy=%s\n",
		proxyAddr, proxyAddr, proxyAddr,
	)
}

// UpdateUIStage 记录 UI 加载阶段并向前端发送阶段事件
// 对应原: update_ui_stage
func (a *ConfigAPI) UpdateUIStage(stage string) {
	utils.Log().Info("UI stage: " + stage)
	emitEvent("ui:stage", map[string]string{"stage": stage})
}

// GetRuntimeProxyChainConfig 获取指定出口节点的代理链运行时配置
// 对应原: get_runtime_proxy_chain_config
func (a *ConfigAPI) GetRuntimeProxyChainConfig(exitNode string) (string, error) {
	snap := a.mgr.GetRuntime()

	// 在运行时配置中查找匹配的代理链配置
	if snap.Config == nil {
		return "", fmt.Errorf("runtime config not available")
	}

	// 序列化整个运行时配置，找到 exitNode 对应的出口
	data, err := yaml.Marshal(snap.Config)
	if err != nil {
		return "", fmt.Errorf("marshal runtime: %w", err)
	}
	_ = exitNode // 当前返回整个运行时配置，后续可按 exitNode 过滤
	return string(data), nil
}

// UpdateProxyChainConfigInRuntime 动态更新代理链配置到运行时（不重启核心）
// 对应原: update_proxy_chain_config_in_runtime
func (a *ConfigAPI) UpdateProxyChainConfigInRuntime(chainConfig map[string]interface{}) error {
	if chainConfig == nil {
		return nil
	}
	// 将代理链配置合并到 clash 基础配置
	if err := a.mgr.PatchClash(chainConfig); err != nil {
		return err
	}
	// 热加载
	if coreManagerRef != nil {
		return coreManagerRef.UpdateConfig()
	}
	return nil
}

// ─── 图标缓存 ─────────────────────────────────────────────────────────────────

// DownloadIconCache 下载代理图标到本地缓存目录
// 对应原: download_icon_cache
func (a *ConfigAPI) DownloadIconCache(iconURL, name string) (string, error) {
	return downloadAndCacheIcon(iconURL, name, utils.Dirs().HomeDir())
}

// downloadAndCacheIcon 下载图标并保存到 icons/ 子目录
func downloadAndCacheIcon(iconURL, name, homeDir string) (string, error) {
	iconsDir := homeDir + "/icons"
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		return "", fmt.Errorf("create icons dir: %w", err)
	}

	ext := ".png"
	if len(iconURL) > 4 && iconURL[len(iconURL)-4:] == ".svg" {
		ext = ".svg"
	}
	destPath := iconsDir + "/" + name + ext

	data, err := fetchURLBytes(iconURL)
	if err != nil {
		return "", fmt.Errorf("download icon: %w", err)
	}

	if err := config.WriteFileAtomic(destPath, data); err != nil {
		return "", fmt.Errorf("save icon: %w", err)
	}

	return destPath, nil
}

// ─── 文件操作（供前端 tauri-shim 调用）─────────────────────────────────────────

// ReadTextFile 读取文本文件内容
func (a *ConfigAPI) ReadTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

// WriteTextFile 写入文本文件
func (a *ConfigAPI) WriteTextFile(path, content string) error {
	return config.WriteFileAtomic(path, []byte(content))
}

// FileExists 检查文件是否存在
func (a *ConfigAPI) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
