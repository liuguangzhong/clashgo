package api

import (
	"clashgo/internal/backup"
	"clashgo/internal/config"
	"clashgo/internal/proxy"
	"clashgo/internal/service"
	"clashgo/internal/utils"
)

// 以下 Init 方法用于在 Wails Startup 阶段注入依赖。
// Wails 在 Startup 之前就调用 Bindings() 获取绑定对象，
// 因此 API 对象必须在 NewApp() 时创建（空壳），
// 在 Startup() 时通过 Init 方法注入真正的依赖。

// InitConfigAPI 注入 config.Manager 到已存在的 ConfigAPI 实例
func (a *ConfigAPI) Init(mgr *config.Manager) {
	a.mgr = mgr
}

// InitProxyAPI 注入 config.Manager 到已存在的 ProxyAPI 实例
func (a *ProxyAPI) Init(mgr *config.Manager) {
	a.mgr = mgr
}

// InitProfileAPI 注入 config.Manager 到已存在的 ProfileAPI 实例
func (a *ProfileAPI) Init(mgr *config.Manager) {
	a.mgr = mgr
}

// InitSystemAPI 初始化 SystemAPI 的内部依赖
func (a *SystemAPI) Init() {
	a.sysProxy = proxy.NewSysProxy()
	a.mihomoSrv = "127.0.0.1:9097"
}

// InitBackupAPI 注入 config.Manager 并创建内部 backuper
func (a *BackupAPI) Init(mgr *config.Manager) {
	a.mgr = mgr
	a.local = backup.NewLocalBackuper(utils.Dirs(), mgr)
	a.webdav = backup.NewWebDAVBackuper(mgr, a.local)
}

// InitServiceAPI 初始化 ServiceAPI 的内部依赖
func (a *ServiceAPI) Init() {
	a.mgr = service.New()
}
