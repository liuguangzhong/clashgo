package api

import (
	"clashgo/internal/config"
)

// ConfigAPI 配置相关 API（绑定到前端）
// 对应原 src-tauri/src/cmd/ 中 config 相关命令
type ConfigAPI struct {
	mgr *config.Manager
}

func NewConfigAPI(mgr *config.Manager) *ConfigAPI {
	return &ConfigAPI{mgr: mgr}
}

// eventEmitter 由 app.go 在 Startup 时注入，用于 API 层向前端推送事件
// 设计：API 层不持有 Wails context，通过此回调间接 emit
var eventEmitter func(event string, data interface{})

// SetEventEmitter 由 app.go 注入 Wails EventsEmit 封装
func SetEventEmitter(fn func(event string, data interface{})) {
	eventEmitter = fn
}

// emitEvent 安全发射事件（emitter 未注入时静默忽略）
func emitEvent(event string, data interface{}) {
	if eventEmitter != nil {
		eventEmitter(event, data)
	}
}

// GetVergeConfig 获取当前 IVerge 配置
// 对应原: cmd::get_verge_config
func (a *ConfigAPI) GetVergeConfig() config.IVerge {
	return a.mgr.GetVerge()
}

// GetVergeRaw 返回 IVerge 指针（内部使用，不导出给 Wails）
func (a *ConfigAPI) GetVergeRaw() config.IVerge {
	return a.mgr.GetVerge()
}

// PatchVergeConfig 修改 IVerge 配置（patch 语义：只更新非nil字段）
// 对应原: cmd::patch_verge_config
func (a *ConfigAPI) PatchVergeConfig(patch config.IVerge) error {
	return a.mgr.PatchVerge(patch)
}

// GetClashInfo 获取 Clash 连接信息（端口、secret 等）
// 对应原: cmd::get_clash_info
func (a *ConfigAPI) GetClashInfo() config.ClashInfo {
	clash := a.mgr.GetClash()
	verge := a.mgr.GetVerge()

	mixedPort := uint16(7897)
	if verge.VergeMixedPort != nil {
		mixedPort = *verge.VergeMixedPort
	} else if p, ok := clash["mixed-port"].(int); ok {
		mixedPort = uint16(p)
	}

	socksPort := uint16(7898)
	if verge.VergeSocksPort != nil {
		socksPort = *verge.VergeSocksPort
	}

	httpPort := uint16(7899)
	if verge.VergePort != nil {
		httpPort = *verge.VergePort
	}

	server := "127.0.0.1:9097"
	if ctrl, ok := clash["external-controller"].(string); ok && ctrl != "" {
		server = ctrl
	}

	var secret *string
	if s, ok := clash["secret"].(string); ok && s != "" {
		secret = &s
	}

	return config.ClashInfo{
		MixedPort: mixedPort,
		SocksPort: socksPort,
		HTTPPort:  httpPort,
		Server:    server,
		Secret:    secret,
	}
}

// PatchClashConfig 修改 Clash 基础配置
// 对应原: cmd::patch_clash_config
func (a *ConfigAPI) PatchClashConfig(patch map[string]interface{}) error {
	return a.mgr.PatchClash(patch)
}

// PatchClashMode 切换代理模式（rule/global/direct）并立即热加载
// 对应原: cmd::patch_clash_mode
func (a *ConfigAPI) PatchClashMode(mode string) error {
	if err := a.mgr.PatchClash(map[string]interface{}{"mode": mode}); err != nil {
		return err
	}
	// 切换模式后立即热加载，让 Mihomo 生效
	if coreManagerRef != nil {
		return coreManagerRef.UpdateConfig()
	}
	return nil
}

// GetRuntimeConfig 获取运行时生成的完整配置快照
// 对应原: cmd::get_runtime_config
func (a *ConfigAPI) GetRuntimeConfig() config.RuntimeSnapshot {
	return a.mgr.GetRuntime()
}

// GetRuntimeYAML 获取运行时 YAML 文本内容
func (a *ConfigAPI) GetRuntimeYAML() (string, error) {
	return runtimeYAMLText(a.mgr)
}

// GetRuntimeLogs 获取增强流水线的脚本执行日志
// 对应原: cmd::get_runtime_logs
func (a *ConfigAPI) GetRuntimeLogs() map[string][][2]string {
	return a.mgr.GetRuntime().ChainLogs
}
