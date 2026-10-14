package api

import (
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

// authFloorJitter 给认证步骤加一段随机：等待落在 AuthFloor .. AuthFloor+jitter 之间，
// 成功与失败、这一次与下一次都对不齐，快慢里读不出东西。
const authFloorJitter = 200 * time.Millisecond

type authView struct {
	OK    bool   `json:"ok"`
	Token string `json:"token,omitempty"`
}

// POST /api/auth：唯一会见到原始 token 的接口。
// 验过之后签发一条会话 token 回去，之后所有 /api/* 只认这条会话，原始 token 不必再上路。
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
	passed := token != nil && s.tokenMatches(*token)
	if token != nil && !passed {
		key = i18n.KeyHTTPTokenBad
	}
	session, err := s.issueSession(passed)
	s.waitAuthFloor(started)
	s.logLogin(r, passed)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if !passed {
		s.failAuth(w, r, key)
		return
	}
	httpx.JSON(w, http.StatusOK, authView{OK: true, Token: session})
}

// requireAuth 包住整个 /api/*：只认认证接口签出去的会话 token。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			s.failAuth(w, r, i18n.KeyHTTPAuthRequired)
			return
		}
		// SessionSigner 由接线时注入，缺了就是接线错了：失败关闭，不放行
		if s.opts.SessionSigner == nil || !s.opts.SessionSigner.Valid(token) {
			s.failAuth(w, r, i18n.KeyHTTPTokenBad)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// issueSession 只在验过之后签会话；没验过不给，随机源坏了就报 500。
func (s *Server) issueSession(passed bool) (string, error) {
	if !passed || s.opts.SessionSigner == nil {
		return "", nil
	}
	return s.opts.SessionSigner.Issue()
}

// waitAuthFloor 把认证步骤补足到「下限 + 随机抖动」，成功与失败走同一条路。
func (s *Server) waitAuthFloor(started time.Time) {
	if wait := s.opts.AuthFloor + rand.N(authFloorJitter) - time.Since(started); wait > 0 {
		time.Sleep(wait)
	}
}

// failAuth 是 401 的唯一出口：带 WWW-Authenticate，文案按请求语言渲染。
func (s *Server) failAuth(w http.ResponseWriter, r *http.Request, key i18n.Key) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	httpx.Fail(w, http.StatusUnauthorized, i18n.T(langOf(r), key))
}

// logLogin 登录成败各记一条审计日志：只给客户端 IP 与结果，token 一个字符都不进日志。
func (s *Server) logLogin(r *http.Request, passed bool) {
	s.opts.Logger.Info("login", "client_ip", httpx.ClientIP(r, s.opts.TrustedProxies), "ok", passed)
}

// tokenMatches 拿请求里的原始 token 散一遍和内存里那份摘要比，恒定时间；
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
