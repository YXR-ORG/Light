package handler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
	"light-ai/internal/storage"
)

type BackupHandler struct{}

func NewBackupHandler() *BackupHandler { return &BackupHandler{} }

type WebDAVConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Path     string `json:"path"`
}

func (h *BackupHandler) SaveConfig(url, username, password, path string) error {
	if path == "" {
		path = "/Light/"
	}
	if err := storage.SetSetting("webdav_url", url); err != nil {
		return err
	}
	if err := storage.SetSetting("webdav_username", username); err != nil {
		return err
	}
	if password != "" {
		if err := storage.SetSetting("webdav_password", password); err != nil {
			return err
		}
	}
	return storage.SetSetting("webdav_path", path)
}

func (h *BackupHandler) GetConfig() (WebDAVConfig, error) {
	url, _ := storage.GetSetting("webdav_url")
	username, _ := storage.GetSetting("webdav_username")
	path, _ := storage.GetSetting("webdav_path")
	if path == "" {
		path = "/Light/"
	}
	return WebDAVConfig{URL: url, Username: username, Path: path}, nil
}

func newClient() (*gowebdav.Client, string, error) {
	url, _ := storage.GetSetting("webdav_url")
	if url == "" {
		return nil, "", fmt.Errorf("WebDAV 未配置，请先在设置中填写服务器地址")
	}
	username, _ := storage.GetSetting("webdav_username")
	password, _ := storage.GetSetting("webdav_password")
	path, _ := storage.GetSetting("webdav_path")
	if path == "" {
		path = "/Light/"
	}
	c := gowebdav.NewClient(url, username, password)
	return c, path, nil
}

func (h *BackupHandler) Backup() error {
	c, remotePath, err := newClient()
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".wails-chat", "chat.db")

	f, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer f.Close()

	// Ensure remote directory exists
	_ = c.MkdirAll(remotePath, 0755)

	filename := "chat-" + time.Now().Format("20060102-150405") + ".db"
	if !strings.HasSuffix(remotePath, "/") {
		remotePath += "/"
	}
	if err := c.WriteStream(remotePath+filename, f, 0644); err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	return nil
}

type BackupFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

func (h *BackupHandler) ListBackups() ([]BackupFile, error) {
	c, remotePath, err := newClient()
	if err != nil {
		return nil, err
	}

	files, err := c.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	var result []BackupFile
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".db") {
			result = append(result, BackupFile{
				Name:    f.Name(),
				Size:    f.Size(),
				ModTime: f.ModTime().Format("2006-01-02 15:04:05"),
			})
		}
	}
	// Sort descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name > result[j].Name
	})
	return result, nil
}

func (h *BackupHandler) DeleteBackup(filename string) error {
	c, remotePath, err := newClient()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(remotePath, "/") {
		remotePath += "/"
	}
	if err := c.Remove(remotePath + filename); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	return nil
}

func (h *BackupHandler) Restore(filename string) error {
	c, remotePath, err := newClient()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(remotePath, "/") {
		remotePath += "/"
	}

	reader, err := c.ReadStream(remotePath + filename)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	defer reader.Close()

	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".wails-chat", "chat.db")

	// Backup current db before overwriting
	_ = os.Rename(dbPath, dbPath+".bak")

	dest, err := os.Create(dbPath)
	if err != nil {
		_ = os.Rename(dbPath+".bak", dbPath)
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, reader); err != nil {
		dest.Close()
		_ = os.Remove(dbPath)
		_ = os.Rename(dbPath+".bak", dbPath)
		return fmt.Errorf("写入失败: %w", err)
	}
	return nil
}
