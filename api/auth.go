package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

type authView struct {
	OK bool `json:"ok"`
}

// POST /api/auth：拿请求体里的 token 换一个「可以进站」的答复。
// 这是唯一不要求 Authorization 头的 /api 接口，认证页用它验用户粘进来的 token。
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	token, err := bodyString(raw, "token")
	if err != nil {
		writeError(w, r, err)
		return
	}

	started := time.Now()
	key := i18n.KeyHTTPAuthRequired
	passed := false
	if token != nil {
		if passed = s.tokenMatches(*token); !passed {
			key = i18n.KeyHTTPTokenBad
		}
	}
	s.waitAuthFloor(started)
	if !passed {
		s.failAuth(w, r, key)
		return
	}
	httpx.JSON(w, http.StatusOK, authView{OK: true})
}

// waitAuthFloor 把认证步骤补足到下限：成功与失败都一样，快慢里读不出东西。
func (s *Server) waitAuthFloor(started time.Time) {
	if wait := s.opts.AuthFloor - time.Since(started); wait > 0 {
		time.Sleep(wait)
	}
}

// requireAuth 包住整个 /api/*：只认 Authorization: Bearer <AUTH_TOKEN>。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			s.failAuth(w, r, i18n.KeyHTTPAuthRequired)
			return
		}
		if !s.tokenMatches(token) {
			s.failAuth(w, r, i18n.KeyHTTPTokenBad)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// failAuth 是 401 的唯一出口：带 WWW-Authenticate，文案按请求语言渲染。
func (s *Server) failAuth(w http.ResponseWriter, r *http.Request, key i18n.Key) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	httpx.Fail(w, http.StatusUnauthorized, i18n.T(langOf(r), key))
}

// tokenMatches 拿请求里的 token 再散一遍和内存里那份摘要比，恒定时间；
// 摘要不合法（启动时就会挡住，属于不该发生的事）就当不匹配。
func (s *Server) tokenMatches(candidate string) bool {
	matched, err := core.VerifyToken(candidate, s.opts.AuthTokenHash)
	return err == nil && matched
}

// bearerToken 取出 Authorization 头里 Bearer 后面的值；格式不对或为空都算「没带」。
func bearerToken(r *http.Request) (string, bool) {
	scheme, value, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(value)
	return token, token != ""
}
