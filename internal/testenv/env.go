// Package testenv 是测试脚手架：临时数据目录、临时配置文件、现成的测试服务。
// 测试一律不许碰真实的数据目录。
package testenv

import (
	"bytes"
	"encoding/json"
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
	"github.com/hm-service/scope-status/core"
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

// DefaultAuthToken 是测试实例默认的 Bearer token；Env.Client 会自动带上它。
const DefaultAuthToken = "test-token-0123456789"

type Options struct {
	StreamInterval time.Duration
	ReadyzToken    string
	AuthToken      string
	// AuthFloor 默认压到 1ms：认证步骤的下限本身另有专门用例，别让每个用例都真等 1 秒
	AuthFloor      time.Duration
	MaxUploadBytes int64
	Static         fs.FS
}

type Env struct {
	T            *testing.T
	DataDir      string
	SourceFile   string
	App          *app.App
	Server       *httptest.Server
	BaseURL      string
	AuthToken    string
	SessionToken string
	Client       *http.Client
	RawClient    *http.Client
	Logger       *slog.Logger
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
		StreamInterval: opts.StreamInterval,
		MaxUploadBytes: maxUpload,
	})
	if err != nil {
		t.Fatalf("建应用容器失败：%v", err)
	}
	authToken := opts.AuthToken
	if authToken == "" {
		authToken = DefaultAuthToken
	}
	// 服务端只拿摘要，测试自己留着明文发请求
	authTokenHash, err := core.HashToken(authToken)
	if err != nil {
		t.Fatalf("散列测试 token 失败：%v", err)
	}
	// 会话表：测试实例也是启动时生成一把随机密钥，重启（新实例）即失效
	sessions, err := core.NewSessions()
	if err != nil {
		t.Fatalf("建会话表失败：%v", err)
	}
	static := opts.Static
	if static == nil {
		static = Static()
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	authFloor := opts.AuthFloor
	if authFloor == 0 {
		authFloor = time.Millisecond
	}
	handler := api.New(api.Options{
		App:                  application,
		Static:               static,
		Logger:               logger,
		TrustedProxies:       nil,
		ReadyzToken:          opts.ReadyzToken,
		AuthTokenHash:        authTokenHash,
		Sessions:             sessions,
		AuthFloor:            authFloor,
		MaxAPIConcurrency:    8,
		MaxStaticConcurrency: 4,
	}).Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	const timeout = 10 * time.Second
	// 会话只能登录换：测试也走一遍 POST /api/auth，别去开后门
	sessionToken := login(t, server.URL, authToken)
	return &Env{
		T:            t,
		DataDir:      dataDir,
		SourceFile:   sourceFile,
		App:          application,
		Server:       server,
		BaseURL:      server.URL,
		AuthToken:    authToken,
		SessionToken: sessionToken,
		Client:       &http.Client{Timeout: timeout, Transport: &authTransport{token: sessionToken}},
		RawClient:    &http.Client{Timeout: timeout},
		Logger:       logger,
	}
}

// login 拿原始 token 换一条会话 token。
func login(t *testing.T, baseURL, authToken string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"token": authToken})
	if err != nil {
		t.Fatalf("造登录请求失败：%v", err)
	}
	response, err := http.Post(baseURL+"/api/auth", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("登录失败：%v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("登录状态码 = %d", response.StatusCode)
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Token == "" {
		t.Fatalf("登录响应里没有会话 token：%v", err)
	}
	return payload.Token
}

// authTransport 给每个请求补上 Authorization: Bearer；调用方自己设了就不覆盖。
type authTransport struct {
	token string
}

func (t *authTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	if clone.Header.Get("Authorization") == "" {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	return http.DefaultTransport.RoundTrip(clone)
}

// Static 是一份最小的前端产物，够静态文件用例跑。
func Static() fs.FS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<!doctype html><title>test</title>")},
		"auth.html":     &fstest.MapFile{Data: []byte("<!doctype html><title>auth</title>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('test')")},
	}
}
