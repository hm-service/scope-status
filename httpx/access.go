package httpx

import (
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

// AccessLog 一条请求一行 JSON；探针路径不记，免得被 kubelet 刷屏。
type AccessLog struct {
	Logger    *slog.Logger
	Trusted   []netip.Prefix
	SkipPaths map[string]bool
}

func (l *AccessLog) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.SkipPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		recorder := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		l.Logger.Info("access",
			"client_ip", ClientIP(r, l.Trusted),
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", float64(time.Since(started).Microseconds())/1000,
			"lang", string(i18n.Negotiate(r.Header.Get("Accept-Language"))),
		)
	})
}

// recorder 记下状态码与字节数；SSE 要能 Flush，所以把 Flusher 透传下去。
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(body []byte) (int, error) {
	written, err := r.ResponseWriter.Write(body)
	r.bytes += int64(written)
	return written, err
}

func (r *recorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
