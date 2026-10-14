package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/hm-service/scope-status/internal/testenv"
)

type authOK struct {
	OK bool `json:"ok"`
}

// /api/* 一律要 Bearer token：缺、错、换个 scheme 都是 401 + WWW-Authenticate。
func TestAPINeedsBearerToken(t *testing.T) {
	env := testenv.New(t, sourceWith(listenTCP(t)))

	missing := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "")
	if missing.status != http.StatusUnauthorized {
		t.Fatalf("没带 token 应该 401，拿到 %d（%s）", missing.status, missing.body)
	}
	if got := missing.header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
	if detail := decode[errorBody](t, missing.body).Detail; !strings.Contains(detail, "Bearer token") {
		t.Fatalf("401 文案 = %q", detail)
	}

	wrong := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "", [2]string{"Authorization", "Bearer nope"})
	if wrong.status != http.StatusUnauthorized || !strings.Contains(string(wrong.body), "token 不对") {
		t.Fatalf("token 不对应该 401：%d（%s）", wrong.status, wrong.body)
	}

	other := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "", [2]string{"Authorization", "Basic YWJj"})
	if other.status != http.StatusUnauthorized {
		t.Fatalf("别的 scheme 应该 401，拿到 %d", other.status)
	}

	ok := callWith(t, env, env.RawClient, http.MethodGet, "/api/status?force=1", "", [2]string{"Authorization", "Bearer " + env.AuthToken})
	if ok.status != http.StatusOK {
		t.Fatalf("token 对了应该 200，拿到 %d（%s）", ok.status, ok.body)
	}
}

// 每条 /api 路由、每个方法都要挡；未知 /api 路径也先挡再说。
func TestEveryAPIRouteIsGuarded(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	id := "message-2026-01-01-00-00-00-0000-abc123"
	requests := [][2]string{
		{http.MethodGet, "/api/status"},
		{http.MethodGet, "/api/events"},
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/reload"},
		{http.MethodPost, "/api/reload"},
		{http.MethodGet, "/api/messages"},
		{http.MethodPost, "/api/messages"},
		{http.MethodPut, "/api/messages/files?name=a.txt"},
		{http.MethodGet, "/api/messages/files/" + strings.Repeat("a", 64) + "?id=" + id},
		{http.MethodGet, "/api/messages/" + id},
		{http.MethodPatch, "/api/messages/" + id},
		{http.MethodPost, "/api/messages/" + id + "/delete"},
		{http.MethodGet, "/api/nope"},
	}
	for _, request := range requests {
		response := callWith(t, env, env.RawClient, request[0], request[1], "")
		if response.status != http.StatusUnauthorized {
			t.Errorf("%s %s 没带 token 应该 401，拿到 %d", request[0], request[1], response.status)
		}
	}
}

// POST /api/auth 是登录口：拿请求体里的 token 换 ok，不对就是 401。
func TestAuthEndpoint(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)

	ok := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", `{"token":"`+env.AuthToken+`"}`)
	if ok.status != http.StatusOK || !decode[authOK](t, ok.body).OK {
		t.Fatalf("token 对了应该 200：%d（%s）", ok.status, ok.body)
	}

	wrong := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", `{"token":"nope"}`)
	if wrong.status != http.StatusUnauthorized || !strings.Contains(string(wrong.body), "token 不对") {
		t.Fatalf("token 不对应该 401：%d（%s）", wrong.status, wrong.body)
	}
	if got := wrong.header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q", got)
	}

	for name, body := range map[string]string{"没有 token 字段": `{}`, "空 body": "", "token 不是字符串": `{"token":1}`} {
		response := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", body)
		want := http.StatusUnauthorized
		if name == "token 不是字符串" {
			want = http.StatusBadRequest
		}
		if response.status != want {
			t.Errorf("%s：状态码 = %d，想要 %d（%s）", name, response.status, want, response.body)
		}
	}

	if broken := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", "{"); broken.status != http.StatusBadRequest {
		t.Fatalf("坏 JSON 应该 400，拿到 %d", broken.status)
	}

	method := callWith(t, env, env.RawClient, http.MethodGet, "/api/auth", "")
	if method.status != http.StatusMethodNotAllowed || method.header.Get("Allow") != "POST" {
		t.Fatalf("GET /api/auth 应该 405 + Allow: POST，拿到 %d %q", method.status, method.header.Get("Allow"))
	}
}

// 401 的文案跟着 Accept-Language 走。
func TestAuthErrorFollowsLanguage(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	english := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "", [2]string{"Accept-Language", "en-US,en;q=0.9"})
	if !strings.Contains(string(english.body), "bearer token required") {
		t.Fatalf("英文文案 = %q", english.body)
	}
	chinese := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "", [2]string{"Accept-Language", "zh-CN"})
	if !strings.Contains(string(chinese.body), "需要 Bearer token") {
		t.Fatalf("中文文案 = %q", chinese.body)
	}
}

// 认证页与探针不受影响：浏览器要能先打开 /auth，kubelet 要能直接探。
func TestShellAndProbesStayPublic(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	for _, path := range []string{"/", "/auth", "/assets/app.js", "/livez", "/readyz"} {
		response := callWith(t, env, env.RawClient, http.MethodGet, path, "")
		if response.status != http.StatusOK {
			t.Errorf("%s 不该要 token，拿到 %d（%s）", path, response.status, response.body)
		}
	}
	page := callWith(t, env, env.RawClient, http.MethodGet, "/auth", "")
	if !strings.Contains(string(page.body), "<title>auth</title>") {
		t.Fatalf("/auth 应该是认证页：%q", page.body)
	}
}
