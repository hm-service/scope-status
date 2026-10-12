// Package api 是 HTTP 层：路由、中间件、语言协商、探针、静态文件与 SSE。
package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/hm-service/scope-status/app"
	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

type Options struct {
	App                  *app.App
	Static               fs.FS
	Logger               *slog.Logger
	TrustedProxies       []netip.Prefix
	ReadyzToken          string
	MaxAPIConcurrency    int
	MaxStaticConcurrency int
}

type Server struct {
	opts      Options
	apiSem    chan struct{}
	staticSem chan struct{}
}

func New(opts Options) *Server {
	return &Server{
		opts:      opts,
		apiSem:    make(chan struct{}, max(1, opts.MaxAPIConcurrency)),
		staticSem: make(chan struct{}, max(1, opts.MaxStaticConcurrency)),
	}
}

func (s *Server) Handler() http.Handler {
	s.opts.App.Monitor.SetSnapshotView(SnapshotView)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/reload", s.handleReload)
	mux.HandleFunc("POST /api/reload", s.handleReload)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/messages", s.handleMessageList)
	mux.HandleFunc("POST /api/messages", s.handleMessageCreate)
	mux.HandleFunc("PUT /api/messages/files", s.handleFileUpload)
	mux.HandleFunc("GET /api/messages/files/{sha256}", s.handleFileDownload)
	mux.HandleFunc("POST /api/messages/{message_id}/unlock", s.handleMessageUnlock)
	mux.HandleFunc("POST /api/messages/{message_id}/delete", s.handleMessageDelete)
	mux.HandleFunc("GET /api/messages/{message_id}", s.handleMessageGet)
	mux.HandleFunc("PATCH /api/messages/{message_id}", s.handleMessageUpdate)
	mux.HandleFunc("GET /livez", s.handleLivez)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("/", s.handleStatic)

	var handler http.Handler = mux
	handler = withLang(handler)
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
			httpx.Text(w, http.StatusServiceUnavailable, i18n.T(langOf(r), i18n.KeyHTTPBusy)+"\n")
		}
	})
}

func isAPI(path string) bool {
	return strings.HasPrefix(path, "/api/") || path == "/livez" || path == "/readyz"
}

// logBusiness 日志里只给文案键与参数，渲染成哪种语言留给 HTTP 响应。
func (s *Server) logBusiness(level slog.Level, message string, r *http.Request, err error) {
	attributes := []any{"ip", httpx.ClientIP(r, s.opts.TrustedProxies)}
	var business *core.Error
	if errors.As(err, &business) {
		attributes = append(attributes, "key", string(business.Key), "args", business.Args, "status", business.Status)
	} else {
		attributes = append(attributes, "error", err.Error())
	}
	s.opts.Logger.Log(r.Context(), level, message, attributes...)
}
