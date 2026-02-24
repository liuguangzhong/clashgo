package api

import (
	"clashgo/internal/service"
)

// ServiceAPI 系统服务管理 API（Windows TUN 模式提权）
type ServiceAPI struct {
	mgr *service.Manager
}

func NewServiceAPI() *ServiceAPI {
	return &ServiceAPI{mgr: service.New()}
}

// InstallService 安装系统服务（需要管理员权限，通过 PowerShell 提权）
// 对应原: install_service
func (a *ServiceAPI) InstallService() error {
	return a.mgr.Install()
}

// UninstallService 卸载系统服务
// 对应原: uninstall_service
func (a *ServiceAPI) UninstallService() error {
	return a.mgr.Uninstall()
}

// ReinstallService 重装系统服务（先卸载再安装）
// 对应原: reinstall_service
func (a *ServiceAPI) ReinstallService() error {
	return a.mgr.Reinstall()
}

// RepairService 强制修复系统服务
// 对应原: repair_service
func (a *ServiceAPI) RepairService() error {
	return a.mgr.Repair()
}

// IsServiceAvailable 检查系统服务是否已安装并可用
// 对应原: is_service_available
func (a *ServiceAPI) IsServiceAvailable() bool {
	return a.mgr.IsAvailable()
}

// GetServiceStatus 获取服务详细状态
func (a *ServiceAPI) GetServiceStatus() service.ServiceStatus {
	return a.mgr.QueryStatus()
}

// InvokeUWPTool 调用 UWP 代理豁免工具（Windows Only）
// 对应原: invoke_uwp_tool
func (a *ServiceAPI) InvokeUWPTool() error {
	return service.InvokeUWPTool()
}
