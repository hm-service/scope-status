package api

import (
	"net/http"
	"strings"

	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

// apiRoute 是 /api/* 的一条路由：模式里 {name} 是路径参数，方法不匹配就是 405。
type apiRoute struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

func (s *Server) routes() []apiRoute {
	return []apiRoute{
		{http.MethodPost, "/api/auth", s.handleAuth},
		{http.MethodGet, "/api/status", s.handleStatus},
		{http.MethodGet, "/api/events", s.handleEvents},
		{http.MethodGet, "/api/reload", s.handleReload},
		{http.MethodPost, "/api/reload", s.handleReload},
		{http.MethodGet, "/api/health", s.handleHealth},
		{http.MethodGet, "/api/messages", s.handleMessageList},
		{http.MethodPost, "/api/messages", s.handleMessageCreate},
		{http.MethodPut, "/api/messages/files", s.handleFileUpload},
		{http.MethodGet, "/api/messages/files/{sha256}", s.handleFileDownload},
		{http.MethodGet, "/api/messages/{message_id}", s.handleMessageGet},
		{http.MethodPatch, "/api/messages/{message_id}", s.handleMessageUpdate},
		{http.MethodPost, "/api/messages/{message_id}/delete", s.handleMessageDelete},
	}
}

// serveAPI 自己按段匹配路由：ServeMux 的通配模式在 /api/messages/* 这种形状上互相不可比
// （files/{sha256} 与 {message_id}/delete 会 panic），顺带也能把 Allow 说准。
// 匹配先字面量后通配：/api/messages/files 同时能对上 PUT 上传口和 {message_id}，
// 字面量赢，否则那条路径的 Allow 会变成一锅粥。
func (s *Server) serveAPI() http.Handler {
	literal, wildcard := splitRoutes(s.routes())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		for _, group := range [2][]apiRoute{literal, wildcard} {
			allow, handled := dispatch(group, w, r, method)
			if handled {
				return
			}
			if allow != "" {
				w.Header().Set("Allow", allow)
				httpx.Fail(w, http.StatusMethodNotAllowed, i18n.T(langOf(r), i18n.KeyHTTPMethodNotAllowed, r.Method))
				return
			}
		}
		httpx.Fail(w, http.StatusNotFound, i18n.T(langOf(r), i18n.KeyHTTPNotFound, r.URL.Path))
	})
}

// dispatch 在一组路由里找匹配：命中且方法一致就交给处理函数，命中但方法不一致返回 Allow。
func dispatch(group []apiRoute, w http.ResponseWriter, r *http.Request, method string) (string, bool) {
	allowed := make([]string, 0, 4)
	for _, route := range group {
		params, ok := matchSegments(route.pattern, r.URL.Path)
		if !ok {
			continue
		}
		allowed = append(allowed, route.method)
		if route.method != method {
			continue
		}
		for name, value := range params {
			r.SetPathValue(name, value)
		}
		route.handler(w, r)
		return "", true
	}
	if len(allowed) == 0 {
		return "", false
	}
	return allowHeader(allowed), false
}

func splitRoutes(routes []apiRoute) (literal, wildcard []apiRoute) {
	for _, route := range routes {
		if strings.Contains(route.pattern, "{") {
			wildcard = append(wildcard, route)
			continue
		}
		literal = append(literal, route)
	}
	return literal, wildcard
}

// serveProbe 把 /livez /readyz 收成只允许 GET（HEAD 由 Go 的语义一并接受），别的给 405。
func (s *Server) serveProbe(get http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			httpx.Fail(w, http.StatusMethodNotAllowed, i18n.T(langOf(r), i18n.KeyHTTPMethodNotAllowed, r.Method))
			return
		}
		get(w, r)
	})
}

// allowHeader 按 Go 的语义把 GET 写成 "GET, HEAD"，其余方法按固定顺序列出来。
func allowHeader(methods []string) string {
	has := make(map[string]bool, len(methods))
	for _, method := range methods {
		has[method] = true
	}
	parts := make([]string, 0, len(methods)+1)
	if has[http.MethodGet] {
		parts = append(parts, "GET", "HEAD")
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if has[method] {
			parts = append(parts, method)
		}
	}
	return strings.Join(parts, ", ")
}

func matchSegments(pattern, path string) (map[string]string, bool) {
	patternSegments := strings.Split(strings.Trim(pattern, "/"), "/")
	pathSegments := strings.Split(strings.Trim(path, "/"), "/")
	if len(patternSegments) != len(pathSegments) {
		return nil, false
	}
	params := make(map[string]string, 2)
	for index, segment := range patternSegments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			params[segment[1:len(segment)-1]] = pathSegments[index]
			continue
		}
		if segment != pathSegments[index] {
			return nil, false
		}
	}
	return params, true
}
