// Package testenv 是测试脚手架：临时数据目录、临时配置文件、现成的测试服务。
// 测试一律不许碰真实的数据目录。
package testenv

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hm-service/scope-status/api"
	"github.com/hm-service/scope-status/app"
)

// DefaultSource 是一份够用的最小配置：一条内网条目（探一个关着的端口）、一个网站。
const DefaultSource = `
title: 测试看板
probe:
  method: tcp
  timeout: 0.5
peers:
  - name: 小明
    kind: friend
    addresses: ["127.0.0.1:1"]
    properties:
      密码: secret
links:
  - name: 示例
    kind: friend
    url: https://example.invalid/
`

type Options struct {
	StreamInterval time.Duration
	ReadyzToken    string
	MaxUploadBytes int64
	Static         fs.FS
}

type Env struct {
	T          *testing.T
	DataDir    string
	SourceFile string
	App        *app.App
	Server     *httptest.Server
	BaseURL    string
	Client     *http.Client
	Logger     *slog.Logger
}

func New(t *testing.T, source string) *Env {
	t.Helper()
	return NewWith(t, source, Options{})
}

// NewWith 造一个临时实例：配置写进临时目录，数据目录也是临时的，服务挂在 httptest 上。
func NewWith(t *testing.T, source string, opts Options) *Env {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("建不下测试数据目录：%v", err)
	}
	sourceFile := filepath.Join(dataDir, app.SourceFileName)
	if err := os.WriteFile(sourceFile, []byte(source), 0o600); err != nil {
		t.Fatalf("写不下测试配置：%v", err)
	}
	maxUpload := opts.MaxUploadBytes
	if maxUpload == 0 {
		maxUpload = 1024 * 1024
	}
	application, err := app.New(app.Config{
		DataDir:        dataDir,
		AuthFloor:      time.Millisecond,
		StreamInterval: opts.StreamInterval,
		MaxUploadBytes: maxUpload,
	})
	if err != nil {
		t.Fatalf("建应用容器失败：%v", err)
	}
	static := opts.Static
	if static == nil {
		static = Static()
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := api.New(api.Options{
		App:                  application,
		Static:               static,
		Logger:               logger,
		TrustedProxies:       nil,
		ReadyzToken:          opts.ReadyzToken,
		MaxAPIConcurrency:    8,
		MaxStaticConcurrency: 4,
	}).Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Env{
		T:          t,
		DataDir:    dataDir,
		SourceFile: sourceFile,
		App:        application,
		Server:     server,
		BaseURL:    server.URL,
		Client:     &http.Client{Timeout: 10 * time.Second},
		Logger:     logger,
	}
}

// Static 是一份最小的前端产物，够静态文件用例跑。
func Static() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><title>test</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('test')")},
	}
}
