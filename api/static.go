package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

// 内网小工具，静态资源一律不缓存：改完刷新就能看到，不用清浏览器缓存。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		httpx.Fail(w, http.StatusNotFound, i18n.T(lang, i18n.KeyHTTPNotFound, r.URL.Path))
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		httpx.Text(w, http.StatusMethodNotAllowed, i18n.T(lang, i18n.KeyHTTPMethodNotAllowed, r.Method)+"\n")
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	// 认证页有固定入口 /auth，产物里的文件名是 auth.html
	if name == "auth" {
		name = "auth.html"
	}
	if !fs.ValidPath(name) {
		s.staticNotFound(w, r)
		return
	}
	entry, err := fs.Stat(s.opts.Static, name)
	if err != nil {
		s.staticNotFound(w, r)
		return
	}
	if entry.IsDir() {
		name = path.Join(name, "index.html")
		if entry, err = fs.Stat(s.opts.Static, name); err != nil {
			s.staticNotFound(w, r)
			return
		}
	}
	file, err := s.opts.Static.Open(name)
	if err != nil {
		s.staticNotFound(w, r)
		return
	}
	defer file.Close()
	content, ok := file.(io.ReadSeeker)
	if !ok {
		httpx.Text(w, http.StatusInternalServerError, i18n.T(lang, i18n.KeyHTTPInternal, name)+"\n")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, path.Base(name), entry.ModTime(), content)
}

func (s *Server) staticNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.Text(w, http.StatusNotFound, i18n.T(langOf(r), i18n.KeyHTTPNotFound, r.URL.Path)+"\n")
}
