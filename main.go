// Scope Status：内网条目 / 服务 / 网站的连通状态看板，外加一个消息面板。
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hm-service/scope-status/api"
	"github.com/hm-service/scope-status/app"
	"github.com/hm-service/scope-status/core"
)

const (
	// 优雅退出的宽限期；超过就强关，并以此退出码表明是被强制的
	shutdownGrace      = 5 * time.Second
	exitShutdownForced = 254
	// 会话 token 的有效期：过期就得重新粘一次 AUTH_TOKEN
	sessionTTL = 30 * 24 * time.Hour
)

// 三个密钥的形状卡死：够长的随机串，字符集只留大小写字母、数字与下划线，写错直接拒绝启动
var (
	authTokenPattern         = regexp.MustCompile(`^[A-Za-z0-9_]{16,128}$`)
	sessionSigningKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]{32,128}$`)
	readyzTokenPattern       = regexp.MustCompile(`^[A-Za-z0-9_]{16,128}$`)
)

var hostPattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

type envConfig struct {
	DataDir              string
	Host                 string
	Port                 int
	ReadyzToken          string
	AuthTokenHash        string
	SessionSigningKey    string
	TrustedProxies       []netip.Prefix
	StreamInterval       time.Duration
	MaxAPIConcurrency    int
	MaxStaticConcurrency int
	MaxUploadBytes       int64
}

func main() {
	os.Exit(run())
}

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		// 访问日志里的时间戳按规范叫 timestamp
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	}))
	slog.SetDefault(logger)

	cfg, err := loadEnv()
	if err != nil {
		startupFailed(err)
		return 1
	}
	application, err := app.New(app.Config{
		DataDir:        cfg.DataDir,
		StreamInterval: cfg.StreamInterval,
		MaxUploadBytes: cfg.MaxUploadBytes,
	})
	if err != nil {
		startupFailed(err)
		return 1
	}
	static, err := fs.Sub(Dist, "frontend/dist")
	if err != nil {
		startupFailed(err)
		return 1
	}
	// 会话签名器：密钥从环境变量来，多副本共用同一把就能互相认会话
	sessions := core.NewSessionSigner([]byte(cfg.SessionSigningKey), sessionTTL)
	handler := api.New(api.Options{
		App:                  application,
		Static:               static,
		Logger:               logger,
		TrustedProxies:       cfg.TrustedProxies,
		ReadyzToken:          cfg.ReadyzToken,
		AuthTokenHash:        cfg.AuthTokenHash,
		SessionSigner:        sessions,
		MaxAPIConcurrency:    cfg.MaxAPIConcurrency,
		MaxStaticConcurrency: cfg.MaxStaticConcurrency,
	}).Handler()

	// 先占住端口再打启动日志：绑不上就直接失败，不留下一句"已启动"
	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		startupFailed(err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.StreamInterval > 0 {
		go application.Monitor.Stream(ctx)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	logger.Info("server started",
		"addr", listener.Addr().String(),
		"data_dir", cfg.DataDir,
		"start_at", time.Now().Format(time.RFC3339),
	)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	var received os.Signal
	select {
	case err := <-serveErr:
		cancel()
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		startupFailed(err)
		return 1
	case received = <-signals:
		// 收到第一次停止信号后就不再受理，避免连按几下打断优雅退出
		signal.Stop(signals)
		logger.Info("shutdown signal received", "signal", received.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown forced", "signal", received.String(), "error", err.Error())
		server.Close()
		return exitShutdownForced
	}
	cancel()
	logger.Info("server shutdown", "signal", received.String(), "stop_at", time.Now().Format(time.RFC3339))
	return 0
}

func startupFailed(err error) {
	var business *core.Error
	if errors.As(err, &business) {
		slog.Error("startup failed", "key", string(business.Key), "args", business.Args, "status", business.Status)
		return
	}
	slog.Error("startup failed", "error", err.Error())
}

func loadEnv() (envConfig, error) {
	port, err := envInt("PORT", 8080)
	if err != nil {
		return envConfig{}, err
	}
	if port < 1 || port > 65535 {
		return envConfig{}, fmt.Errorf("PORT must be within 1-65535, got %d", port)
	}
	host := envString("HOST", "127.0.0.1")
	if _, err := netip.ParseAddr(host); err != nil && !hostPattern.MatchString(host) {
		return envConfig{}, fmt.Errorf("HOST must be an IP or a hostname, got %q", host)
	}
	if timezone := strings.TrimSpace(os.Getenv("TZ")); timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return envConfig{}, fmt.Errorf("TZ %q is not a known timezone: %w", timezone, err)
		}
	}
	readyzToken := strings.TrimSpace(os.Getenv("READYZ_TOKEN"))
	if readyzToken != "" && !readyzTokenPattern.MatchString(readyzToken) {
		return envConfig{}, fmt.Errorf("READYZ_TOKEN must be 16-128 chars of [A-Za-z0-9_], got %d chars", len(readyzToken))
	}
	os.Unsetenv("READYZ_TOKEN")
	// 整个站点的门禁就是这个 token：没设就等于把 /api 全敞开，所以宁可不启动。
	// 读完立刻散列：进程里只留 Argon2id 摘要，之后每次请求拿请求头里那份再散一遍来比。
	// 环境变量也一并清掉，免得 ping 这类子进程把它继承下去。
	authToken := strings.TrimSpace(os.Getenv("AUTH_TOKEN"))
	if authToken == "" {
		return envConfig{}, fmt.Errorf("AUTH_TOKEN must be set")
	}
	if !authTokenPattern.MatchString(authToken) {
		return envConfig{}, fmt.Errorf("AUTH_TOKEN must be 16-128 chars of [A-Za-z0-9_], got %d chars", len(authToken))
	}
	os.Unsetenv("AUTH_TOKEN")
	authTokenHash, err := core.HashToken(authToken)
	if err != nil {
		return envConfig{}, fmt.Errorf("AUTH_TOKEN cannot be hashed: %w", err)
	}
	// 会话靠签名活，认证靠这把密钥而不是 AUTH_TOKEN：换密钥 = 所有会话立刻失效。
	// 签名要用原始密钥，所以它不能像 AUTH_TOKEN 那样只留摘要；同样读完就清掉环境变量。
	sessionSigningKey := strings.TrimSpace(os.Getenv("SESSION_SIGNING_KEY"))
	if sessionSigningKey == "" {
		return envConfig{}, fmt.Errorf("SESSION_SIGNING_KEY must be set")
	}
	if !sessionSigningKeyPattern.MatchString(sessionSigningKey) {
		return envConfig{}, fmt.Errorf("SESSION_SIGNING_KEY must be 32-128 chars of [A-Za-z0-9_], got %d chars", len(sessionSigningKey))
	}
	os.Unsetenv("SESSION_SIGNING_KEY")
	streamSeconds, err := envFloat("STREAM_INTERVAL", 5)
	if err != nil {
		return envConfig{}, err
	}
	if streamSeconds < 0 {
		return envConfig{}, fmt.Errorf("STREAM_INTERVAL must not be negative, got %v", streamSeconds)
	}
	maxAPI, err := envInt("MAX_API_CONCURRENCY", 128)
	if err != nil {
		return envConfig{}, err
	}
	if maxAPI < 1 {
		return envConfig{}, fmt.Errorf("MAX_API_CONCURRENCY must be positive, got %d", maxAPI)
	}
	maxStatic, err := envInt("MAX_STATIC_CONCURRENCY", 64)
	if err != nil {
		return envConfig{}, err
	}
	if maxStatic < 1 {
		return envConfig{}, fmt.Errorf("MAX_STATIC_CONCURRENCY must be positive, got %d", maxStatic)
	}
	maxUpload, err := envInt("MAX_UPLOAD_BYTES", int(core.MaxUploadBytes))
	if err != nil {
		return envConfig{}, err
	}
	if maxUpload < 1 {
		return envConfig{}, fmt.Errorf("MAX_UPLOAD_BYTES must be positive, got %d", maxUpload)
	}
	trusted, err := parsePrefixes(envString("TRUSTED_PROXIES", "127.0.0.1/32,::1/128"))
	if err != nil {
		return envConfig{}, err
	}
	return envConfig{
		// 配置与状态同放一个目录：DATA_DIR 挂到哪，配置就在哪
		DataDir:              envString("DATA_DIR", "/data"),
		Host:                 host,
		Port:                 port,
		ReadyzToken:          readyzToken,
		AuthTokenHash:        authTokenHash,
		SessionSigningKey:    sessionSigningKey,
		TrustedProxies:       trusted,
		StreamInterval:       time.Duration(streamSeconds * float64(time.Second)),
		MaxAPIConcurrency:    maxAPI,
		MaxStaticConcurrency: maxStatic,
		MaxUploadBytes:       int64(maxUpload),
	}, nil
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	return value, nil
}

func envFloat(name string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, raw)
	}
	return value, nil
}

func parsePrefixes(raw string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(trimmed)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES entry %q is not a CIDR: %w", trimmed, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}
