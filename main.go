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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hm-service/scope-status/api"
	"github.com/hm-service/scope-status/app"
	"github.com/hm-service/scope-status/core"
)

type envConfig struct {
	SourceFile           string
	DataDir              string
	Host                 string
	Port                 int
	ReadyzToken          string
	TrustedProxies       []netip.Prefix
	StreamInterval       time.Duration
	MaxAPIConcurrency    int
	MaxStaticConcurrency int
	MaxUploadBytes       int64
}

func main() {
	if err := run(); err != nil {
		var business *core.Error
		if errors.As(err, &business) {
			slog.Error("startup failed", "key", string(business.Key), "args", business.Args, "status", business.Status)
		} else {
			slog.Error("startup failed", "error", err.Error())
		}
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := loadEnv()
	if err != nil {
		return err
	}
	application, err := app.New(app.Config{
		SourceFile:     cfg.SourceFile,
		DataDir:        cfg.DataDir,
		StreamInterval: cfg.StreamInterval,
		MaxUploadBytes: cfg.MaxUploadBytes,
	})
	if err != nil {
		return err
	}
	static, err := fs.Sub(Dist, "frontend/dist")
	if err != nil {
		return err
	}

	handler := api.New(api.Options{
		App:                  application,
		Static:               static,
		Logger:               logger,
		TrustedProxies:       cfg.TrustedProxies,
		ReadyzToken:          cfg.ReadyzToken,
		MaxAPIConcurrency:    cfg.MaxAPIConcurrency,
		MaxStaticConcurrency: cfg.MaxStaticConcurrency,
	}).Handler()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if cfg.StreamInterval > 0 {
		go application.Monitor.Stream(ctx)
	}

	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	logger.Info("listening",
		"addr", server.Addr,
		"sourceFile", cfg.SourceFile,
		"dataDir", cfg.DataDir,
		"streamIntervalSeconds", cfg.StreamInterval.Seconds(),
		"maxUploadBytes", cfg.MaxUploadBytes,
	)

	serverError := make(chan error, 1)
	go func() { serverError <- server.ListenAndServe() }()
	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

func loadEnv() (envConfig, error) {
	port, err := envInt("PORT", 8080)
	if err != nil {
		return envConfig{}, err
	}
	if port < 1 || port > 65535 {
		return envConfig{}, fmt.Errorf("PORT must be within 1-65535, got %d", port)
	}
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
	maxStatic, err := envInt("MAX_STATIC_CONCURRENCY", 64)
	if err != nil {
		return envConfig{}, err
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
	// 配置与状态同放一个目录：DATA_DIR 挂到哪，配置就在哪
	dataDir := envString("DATA_DIR", "/data")
	return envConfig{
		SourceFile:           envString("SOURCE_FILE", filepath.Join(dataDir, "source.yaml")),
		DataDir:              dataDir,
		Host:                 envString("HOST", "127.0.0.1"),
		Port:                 port,
		ReadyzToken:          os.Getenv("READYZ_TOKEN"),
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
