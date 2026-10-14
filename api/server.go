// Package api 是 HTTP 层：路由、中间件、语言协商、探针、静态文件与 SSE。
package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/hm-service/scope-status/app"
	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

// DefaultAuthFloor 是认证步骤的时间下限：验 token 不能秒回，成功与失败都一样。
// 实际等待是下限加一段随机抖动，见 authFloorJitter。
const DefaultAuthFloor = time.Second

type Options struct {
	App                  *app.App
	Static               fs.FS
	Logger               *slog.Logger
	TrustedProxies       []netip.Prefix
	ReadyzToken          string
	AuthTokenHash        string
	Sessions             *core.Sessions
	AuthFloor            time.Duration
	MaxAPIConcurrency    int
	MaxStaticConcurrency int
}

type Server struct {
	opts      Options
	apiSem    chan struct{}
	staticSem chan struct{}
}

func New(opts Options) *Server {
	if opts.AuthFloor <= 0 {
		opts.AuthFloor = DefaultAuthFloor
	}
	return &Server{
		opts:      opts,
		apiSem:    make(chan struct{}, max(1, opts.MaxAPIConcurrency)),
		staticSem: make(chan struct{}, max(1, opts.MaxStaticConcurrency)),
	}
}

func (s *Server) Handler() http.Handler {
	s.opts.App.Monitor.SetSnapshotView(SnapshotView)

	mux := http.NewServeMux()
	// /api/auth 是登录口，只有它不要 token；其余 /api/* 一律先过 Bearer 鉴权。
	mux.Handle("/api/auth", s.serveAPI())
	mux.Handle("/api/", s.requireAuth(s.serveAPI()))
	mux.Handle("/api", s.requireAuth(s.serveAPI()))
	mux.Handle("/livez", s.serveProbe(s.handleLivez))
	mux.Handle("/readyz", s.serveProbe(s.handleReadyz))
	mux.HandleFunc("/", s.handleStatic)

	var handler http.Handler = mux
	handler = withLang(handler)
	handler = httpx.Security(handler)
	handler = s.limit(handler)
	return s.accessLog(handler)
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	log := &httpx.AccessLog{
		Logger:    s.opts.Logger,
		Trusted:   s.opts.TrustedProxies,
		SkipPaths: map[string]bool{"/livez": true},
	}
	return log.Middleware(next)
}

// limit 给 API 与静态文件各自一个信号量：两边槽位独立，互不挤占。
func (s *Server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		semaphore := s.staticSem
		if isAPI(r.URL.Path) {
			semaphore = s.apiSem
		}
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			next.ServeHTTP(w, r)
		case <-r.Context().Done():
			httpx.Fail(w, http.StatusServiceUnavailable, i18n.T(langOf(r), i18n.KeyHTTPBusy))
		}
	})
}

func isAPI(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/livez" || path == "/readyz"
}

// logBusiness 日志里只给文案键与参数，渲染成哪种语言留给 HTTP 响应。
func (s *Server) logBusiness(level slog.Level, message string, r *http.Request, err error) {
	attributes := []any{"client_ip", httpx.ClientIP(r, s.opts.TrustedProxies)}
	var business *core.Error
	if errors.As(err, &business) {
		attributes = append(attributes, "key", string(business.Key), "args", business.Args, "status", business.Status)
	} else {
		attributes = append(attributes, "error", err.Error())
	}
	s.opts.Logger.Log(r.Context(), level, message, attributes...)
}
