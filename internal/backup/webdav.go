package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"clashgo/internal/config"

	"github.com/emersion/go-webdav"
)

// WebDAVBackuper WebDAV 远程备份管理器
type WebDAVBackuper struct {
	cfgMgr *config.Manager
	local  *LocalBackuper
}

// NewWebDAVBackuper 创建 WebDAV 备份器
func NewWebDAVBackuper(cfgMgr *config.Manager, local *LocalBackuper) *WebDAVBackuper {
	return &WebDAVBackuper{cfgMgr: cfgMgr, local: local}
}

// Upload 创建本地备份并上传到 WebDAV
func (w *WebDAVBackuper) Upload() error {
	client, remotePath, err := w.connect()
	if err != nil {
		return err
	}

	filename, err := w.local.Create()
	if err != nil {
		return fmt.Errorf("create local backup: %w", err)
	}

	backupFilePath := w.local.dirs.BackupDir() + "/" + filename
	data, err := osReadFile(backupFilePath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	ctx := context.Background()
	remoteFilePath := path.Join(remotePath, filename)

	// Client.Create 返回 io.WriteCloser，向其写入数据后关闭
	wc, err := client.Create(ctx, remoteFilePath)
	if err != nil {
		return fmt.Errorf("create remote file: %w", err)
	}
	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write to webdav: %w", err)
	}
	return wc.Close()
}

// Download 从 WebDAV 下载备份并恢复
func (w *WebDAVBackuper) Download(filename string) error {
	client, remotePath, err := w.connect()
	if err != nil {
		return err
	}

	ctx := context.Background()
	remoteFilePath := path.Join(remotePath, filename)

	rc, err := client.Open(ctx, remoteFilePath)
	if err != nil {
		return fmt.Errorf("open remote file: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read remote file: %w", err)
	}

	localPath := w.local.dirs.BackupDir() + "/" + filename
	if err := osWriteFile(localPath, data); err != nil {
		return err
	}
	return w.local.Restore(filename)
}

// ListRemote 列出 WebDAV 上的备份文件
func (w *WebDAVBackuper) ListRemote() ([]RemoteBackupInfo, error) {
	client, remotePath, err := w.connect()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	infos, err := client.ReadDir(ctx, remotePath, false)
	if err != nil {
		return nil, fmt.Errorf("list remote dir: %w", err)
	}

	var result []RemoteBackupInfo
	for _, info := range infos {
		if strings.HasSuffix(info.Path, ".zip") {
			result = append(result, RemoteBackupInfo{
				Filename:  path.Base(info.Path),
				Size:      info.Size,
				CreatedAt: info.ModTime,
			})
		}
	}
	return result, nil
}

// DeleteRemote 删除 WebDAV 上的指定备份文件
// 对应原: delete_webdav_backup
func (w *WebDAVBackuper) DeleteRemote(filename string) error {
	client, remotePath, err := w.connect()
	if err != nil {
		return err
	}
	remoteFilePath := path.Join(remotePath, filename)
	return client.RemoveAll(context.Background(), remoteFilePath)
}

// CopyIconFile 复制图标文件到目标路径
// 对应原: copy_icon_file
func CopyIconFile(srcPath, destPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read icon: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0644)
}

// RemoteBackupInfo WebDAV 文件信息
type RemoteBackupInfo struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// connect 使用 Basic Auth 创建 WebDAV 客户端
func (w *WebDAVBackuper) connect() (*webdav.Client, string, error) {
	verge := w.cfgMgr.GetVerge()

	if verge.WebDAVUrl == nil || *verge.WebDAVUrl == "" {
		return nil, "", fmt.Errorf("WebDAV URL not configured")
	}
	if verge.WebDAVUsername == nil || verge.WebDAVPassword == nil {
		return nil, "", fmt.Errorf("WebDAV credentials not configured")
	}

	rawURL := *verge.WebDAVUrl
	username := *verge.WebDAVUsername
	password := *verge.WebDAVPassword
	remotePath := "/clashgo/"

	base := &http.Client{Timeout: 30 * time.Second}
	httpClient := webdav.HTTPClientWithBasicAuth(base, username, password)

	client, err := webdav.NewClient(httpClient, rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("new webdav client: %w", err)
	}

	// 确保远端目录存在（忽略"已存在"错误）
	_ = client.Mkdir(context.Background(), remotePath)

	return client, remotePath, nil
}
