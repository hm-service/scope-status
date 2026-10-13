package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

// SourceFileName 是配置在数据目录里的固定名字：路径没有第二个来源。
const SourceFileName = "source.yaml"

type Config struct {
	DataDir        string
	StreamInterval time.Duration
	MaxUploadBytes int64
	AuthFloor      time.Duration
}

type App struct {
	SourcePath string
	DataDir    string
	Messages   *MessageService
	Monitor    *Monitor
}

// New 建应用容器：建数据目录、收紧权限、读配置，任何一步不成立就直接失败。
func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	sourcePath := filepath.Join(cfg.DataDir, SourceFileName)
	source, err := store.LoadSource(sourcePath)
	if err != nil {
		return nil, err
	}
	messages := store.NewMessageStore(filepath.Join(cfg.DataDir, "messages"), cfg.MaxUploadBytes, cfg.AuthFloor)
	if err := messages.Ensure(); err != nil {
		return nil, err
	}
	monitor := NewMonitor(source, sourcePath, cfg.StreamInterval)
	return &App{
		SourcePath: sourcePath,
		DataDir:    cfg.DataDir,
		Messages:   NewMessageService(messages, monitor),
		Monitor:    monitor,
	}, nil
}

// Ready 断言依赖真的就绪：配置在、数据目录与数据文件权限对、目录可写。
func (a *App) Ready() error {
	if a.Monitor.Source() == nil {
		return core.ConfigError(i18n.KeyConfigMissing, a.SourcePath)
	}
	if err := checkDirMode(a.DataDir); err != nil {
		return core.ServerError(i18n.KeyHTTPNotReadyCause, err.Error())
	}
	if err := a.Messages.Store().CheckPermissions(); err != nil {
		return core.ServerError(i18n.KeyHTTPNotReadyCause, err.Error())
	}
	probe := filepath.Join(a.DataDir, ".readyz")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return core.ServerError(i18n.KeyHTTPNotReadyCause, err.Error())
	}
	return os.Remove(probe)
}

// checkDirMode 断言目录权限被收紧到 0700。
func checkDirMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("%s mode is %v, want 0700", path, info.Mode().Perm())
	}
	return nil
}
