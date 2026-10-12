package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

type Config struct {
	SourceFile     string
	DataDir        string
	StreamInterval time.Duration
	MaxUploadBytes int64
}

type App struct {
	SourcePath string
	DataDir    string
	Messages   *MessageService
	Monitor    *Monitor
}

// New 建应用容器：读配置、建数据目录、收紧权限，任何一步不成立就直接失败。
func New(cfg Config) (*App, error) {
	source, err := store.LoadSource(cfg.SourceFile)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	messages := store.NewMessageStore(filepath.Join(cfg.DataDir, "messages"), cfg.MaxUploadBytes)
	if err := messages.Ensure(); err != nil {
		return nil, err
	}
	monitor := NewMonitor(source, cfg.SourceFile, cfg.StreamInterval)
	return &App{
		SourcePath: cfg.SourceFile,
		DataDir:    cfg.DataDir,
		Messages:   NewMessageService(messages, monitor),
		Monitor:    monitor,
	}, nil
}

// Ready 断言依赖真的就绪：配置在、数据目录可写。
func (a *App) Ready() error {
	if a.Monitor.Source() == nil {
		return core.ConfigError(i18n.KeyConfigMissing, a.SourcePath)
	}
	probe := filepath.Join(a.DataDir, ".readyz")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return core.ServerError(i18n.KeyHTTPNotReady, err.Error())
	}
	return os.Remove(probe)
}
