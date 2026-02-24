package api

import (
	"fmt"
	"os"
	"path/filepath"

	"clashgo/internal/backup"
	"clashgo/internal/config"
	"clashgo/internal/utils"
)

// BackupAPI 备份管理 API
type BackupAPI struct {
	mgr    *config.Manager
	local  *backup.LocalBackuper
	webdav *backup.WebDAVBackuper
}

func NewBackupAPI(mgr *config.Manager) *BackupAPI {
	local := backup.NewLocalBackuper(utils.Dirs(), mgr)
	wdav := backup.NewWebDAVBackuper(mgr, local)
	return &BackupAPI{mgr: mgr, local: local, webdav: wdav}
}

// ─── 本地备份 ─────────────────────────────────────────────────────────────────

// CreateLocalBackup 创建本地备份，返回备份文件名
// 对应原: create_local_backup
func (a *BackupAPI) CreateLocalBackup() (string, error) {
	return a.local.Create()
}

// GetBackupList 列出所有本地备份
// 对应原: list_local_backup
func (a *BackupAPI) GetBackupList() ([]backup.BackupInfo, error) {
	return a.local.List()
}

// DeleteLocalBackup 删除本地备份
// 对应原: delete_local_backup
func (a *BackupAPI) DeleteLocalBackup(filename string) error {
	return a.local.Delete(filename)
}

// RestoreLocalBackup 从本地备份恢复
// 对应原: restore_local_backup
func (a *BackupAPI) RestoreLocalBackup(filename string) error {
	return a.local.Restore(filename)
}

// ImportLocalBackup 从外部路径导入备份文件到应用备份目录
// 对应原: import_local_backup
func (a *BackupAPI) ImportLocalBackup(sourcePath string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("source path is a directory, expected .zip file")
	}

	filename := filepath.Base(sourcePath)
	destPath := filepath.Join(utils.Dirs().BackupDir(), filename)

	if err := copyFile(sourcePath, destPath); err != nil {
		return "", fmt.Errorf("copy backup: %w", err)
	}
	return filename, nil
}

// ExportLocalBackup 将备份文件导出到用户指定路径
// 对应原: export_local_backup
func (a *BackupAPI) ExportLocalBackup(filename, destPath string) error {
	srcPath := filepath.Join(utils.Dirs().BackupDir(), filename)
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}
	return copyFile(srcPath, destPath)
}

// ─── WebDAV 备份 ──────────────────────────────────────────────────────────────

// SaveWebDAVConfig 保存 WebDAV 连接配置
// 对应原: save_webdav_config
func (a *BackupAPI) SaveWebDAVConfig(url, username, password string) error {
	return a.mgr.PatchVerge(config.IVerge{
		WebDAVUrl:      &url,
		WebDAVUsername: &username,
		WebDAVPassword: &password,
	})
}

// UploadToWebDAV 创建本地备份并上传到 WebDAV
// 对应原: create_webdav_backup
func (a *BackupAPI) UploadToWebDAV() error {
	return a.webdav.Upload()
}

// ListWebDAVFiles 列出 WebDAV 上的备份文件
// 对应原: list_webdav_backup
func (a *BackupAPI) ListWebDAVFiles() ([]backup.RemoteBackupInfo, error) {
	return a.webdav.ListRemote()
}

// DeleteWebDAVBackup 删除 WebDAV 上的指定备份文件
// 对应原: delete_webdav_backup
func (a *BackupAPI) DeleteWebDAVBackup(filename string) error {
	return a.webdav.DeleteRemote(filename)
}

// DownloadFromWebDAV 从 WebDAV 下载并恢复
// 对应原: restore_webdav_backup
func (a *BackupAPI) DownloadFromWebDAV(filename string) error {
	return a.webdav.Download(filename)
}

// ─── 图标 ─────────────────────────────────────────────────────────────────────

// CopyIconFile 复制图标文件（供代理节点显示使用）
// 对应原: copy_icon_file
func (a *BackupAPI) CopyIconFile(srcPath, destPath string) error {
	return backup.CopyIconFile(srcPath, destPath)
}
