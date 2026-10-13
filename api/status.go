package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

type healthView struct {
	Status         string             `json:"status"`
	Friends        int                `json:"friends"`
	Services       int                `json:"services"`
	Links          int                `json:"links"`
	Categories     []string           `json:"categories"`
	ReloadCount    int                `json:"reloadCount"`
	StreamInterval float64            `json:"streamInterval"`
	Subscribers    int                `json:"subscribers"`
	HasSnapshot    bool               `json:"hasSnapshot"`
	LastDurationMs float64            `json:"lastDurationMs"`
	SourcePath     string             `json:"sourcePath"`
	Messages       store.MessageStats `json:"messages"`
	MessagesDir    string             `json:"messagesDir"`
	MaxUploadBytes int64              `json:"maxUploadBytes"`
}

type reloadView struct {
	OK          bool   `json:"ok"`
	Message     string `json:"message"`
	Friends     int    `json:"friends"`
	Services    int    `json:"services"`
	Links       int    `json:"links"`
	ReloadCount int    `json:"reloadCount"`
	SourcePath  string `json:"sourcePath"`
	Snapshot    any    `json:"snapshot"`
}

// GET /api/status：无参数时直接给共享快照（恒定时间），?force=1 强制探一次。
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	force := isTruthy(r.URL.Query().Get("force"))
	snapshot, reused, err := s.opts.App.Monitor.Snapshot(force)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if snapshot == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpx.JSON(w, http.StatusOK, SnapshotView(snapshot, langOf(r), reused))
}

// GET|POST /api/reload：重读数据目录里的配置，然后立刻按新配置探一次。
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	source, err := s.opts.App.Monitor.Reload()
	if err != nil {
		writeError(w, r, err)
		return
	}
	snapshot, err := s.opts.App.Monitor.Refresh()
	if err != nil {
		writeError(w, r, err)
		return
	}
	lang := langOf(r)
	httpx.JSON(w, http.StatusOK, reloadView{
		OK:      true,
		Message: i18n.T(lang, i18n.KeyReloadOK, len(source.Friends), len(source.Services), len(source.Links)),
		Friends: len(source.Friends), Services: len(source.Services), Links: len(source.Links),
		ReloadCount: s.opts.App.Monitor.ReloadCount(),
		SourcePath:  s.opts.App.Monitor.SourcePath(),
		Snapshot:    SnapshotView(snapshot, lang, false),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	monitor := s.opts.App.Monitor
	source := monitor.Source()
	stats, err := s.opts.App.Messages.Stats()
	if err != nil {
		writeError(w, r, err)
		return
	}
	categories := make([]string, 0, len(source.Categories))
	for _, category := range source.Categories {
		categories = append(categories, category.ID)
	}
	httpx.JSON(w, http.StatusOK, healthView{
		Status:         "ok",
		Friends:        len(source.Friends),
		Services:       len(source.Services),
		Links:          len(source.Links),
		Categories:     categories,
		ReloadCount:    monitor.ReloadCount(),
		StreamInterval: monitor.StreamInterval().Seconds(),
		Subscribers:    monitor.Subscribers(),
		HasSnapshot:    monitor.Current() != nil,
		LastDurationMs: monitor.LastDurationMs(),
		SourcePath:     monitor.SourcePath(),
		Messages:       stats,
		MessagesDir:    s.opts.App.Messages.Store().Root(),
		MaxUploadBytes: s.opts.App.Messages.Store().MaxUploadBytes(),
	})
}

// GET /livez：进程活着就算活着，不记日志。
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	httpx.Text(w, http.StatusOK, "ok\n")
}

// GET /readyz：查依赖就绪，并记一条审计日志。
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.opts.ReadyzToken != "" {
		token := r.URL.Query().Get("token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.opts.ReadyzToken)) != 1 {
			s.logBusiness(slog.LevelWarn, "readyz", r, core.Forbidden(i18n.KeyHTTPTokenBad))
			// 探针的失败细节只进后端日志，不回给调用方
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	// 失败原因只进后端日志，不回给调用方
	if err := s.opts.App.Ready(); err != nil {
		s.logBusiness(slog.LevelWarn, "readyz", r, err)
		httpx.Fail(w, http.StatusServiceUnavailable, i18n.T(langOf(r), i18n.KeyHTTPNotReady))
		return
	}
	s.opts.Logger.Info("readyz", "client_ip", httpx.ClientIP(r, s.opts.TrustedProxies), "ok", true)
	httpx.Text(w, http.StatusOK, "ok\n")
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
