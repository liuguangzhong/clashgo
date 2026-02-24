package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clashgo/internal/config"
	"clashgo/internal/utils"
)

// LocalBackuper 本地备份管理器
// 对应原 core/backup.rs 本地备份部分
type LocalBackuper struct {
	dirs   *utils.AppDirs
	cfgMgr *config.Manager
}

// NewLocalBackuper 创建本地备份器
func NewLocalBackuper(dirs *utils.AppDirs, cfgMgr *config.Manager) *LocalBackuper {
	return &LocalBackuper{dirs: dirs, cfgMgr: cfgMgr}
}

// Create 创建备份（将关键配置文件打包为 zip）
// 对应原 cmd::backup_config
func (b *LocalBackuper) Create() (string, error) {
	backupDir := b.dirs.BackupDir()
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// 生成备份文件名：clashgo-backup-2026-02-24T20-18-41.zip
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("clashgo-backup-%s.zip", timestamp)
	zipPath := filepath.Join(backupDir, filename)

	if err := b.createZip(zipPath); err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}

	// 清理旧备份（保留最近10个）
	b.pruneOldBackups(10)

	return filename, nil
}

// Restore 从备份文件恢复
func (b *LocalBackuper) Restore(filename string) error {
	zipPath := filepath.Join(b.dirs.BackupDir(), filename)
	return b.extractZip(zipPath, b.dirs.HomeDir())
}

// List 列出所有本地备份
func (b *LocalBackuper) List() ([]BackupInfo, error) {
	entries, err := os.ReadDir(b.dirs.BackupDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zip") {
			continue
		}
		info, _ := e.Info()
		backups = append(backups, BackupInfo{
			Filename:  e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	return backups, nil
}

// Delete 删除指定备份文件
func (b *LocalBackuper) Delete(filename string) error {
	// 安全检查：防止路径穿越
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("invalid filename")
	}
	path := filepath.Join(b.dirs.BackupDir(), filename)
	return os.Remove(path)
}

// BackupInfo 备份文件信息
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── 私有方法 ─────────────────────────────────────────────────────────────────

// createZip 将关键配置文件打包到 zip
func (b *LocalBackuper) createZip(zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// 需要备份的文件列表
	filesToBackup := []string{
		b.dirs.VergePath(),
		b.dirs.ClashPath(),
		b.dirs.ProfilesPath(),
		b.dirs.DNSConfigPath(),
	}

	for _, src := range filesToBackup {
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if err := addFileToZip(w, src, filepath.Base(src)); err != nil {
			return err
		}
	}

	// 备份 profiles 目录下所有 YAML 文件
	profileFiles, _ := filepath.Glob(filepath.Join(b.dirs.ProfilesDir(), "*.yaml"))
	for _, pf := range profileFiles {
		relPath := filepath.Join("profiles", filepath.Base(pf))
		_ = addFileToZip(w, pf, relPath)
	}

	return nil
}

// extractZip 解压备份到目标目录
func (b *LocalBackuper) extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// 安全：防止路径穿越
		destPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(destPath), 0755)

		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}

		src, err := f.Open()
		if err != nil {
			dst.Close()
			return err
		}

		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

// pruneOldBackups 只保留最近 N 个备份
func (b *LocalBackuper) pruneOldBackups(keepCount int) {
	backups, err := b.List()
	if err != nil || len(backups) <= keepCount {
		return
	}

	// 按时间排序（最旧的在前）
	for i := 0; i < len(backups)-keepCount; i++ {
		_ = b.Delete(backups[i].Filename)
	}
}

// addFileToZip 将单个文件添加到 zip writer
func addFileToZip(w *zip.Writer, srcPath, nameInZip string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	f, err := w.Create(nameInZip)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}
