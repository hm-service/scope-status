package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/api"
	"github.com/hm-service/scope-status/internal/testenv"
)

type authView struct {
	OK    bool   `json:"ok"`
	Token string `json:"token"`
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

	// 原始 token 不是会话：除了登录口，哪儿都不认它
	raw := callWith(t, env, env.RawClient, http.MethodGet, "/api/status", "", [2]string{"Authorization", "Bearer " + env.AuthToken})
	if raw.status != http.StatusUnauthorized {
		t.Fatalf("原始 token 不该当会话用，拿到 %d", raw.status)
	}

	ok := callWith(t, env, env.RawClient, http.MethodGet, "/api/status?force=1", "", [2]string{"Authorization", "Bearer " + env.SessionToken})
	if ok.status != http.StatusOK {
		t.Fatalf("会话 token 应该 200，拿到 %d（%s）", ok.status, ok.body)
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

// POST /api/auth 是登录口：拿请求体里的原始 token 换一条会话 token，不对就是 401。
func TestAuthEndpoint(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)

	ok := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", `{"token":"`+env.AuthToken+`"}`)
	if ok.status != http.StatusOK {
		t.Fatalf("token 对了应该 200：%d（%s）", ok.status, ok.body)
	}
	issued := decode[authView](t, ok.body)
	if !issued.OK || issued.Token == "" {
		t.Fatalf("登录应该给一条会话 token：%+v", issued)
	}
	if strings.Contains(string(ok.body), env.AuthToken) {
		t.Fatalf("响应里不该回显原始 token：%s", ok.body)
	}
	// 签出来的会话立刻可用；两条会话互不影响
	used := callWith(t, env, env.RawClient, http.MethodGet, "/api/health", "", [2]string{"Authorization", "Bearer " + issued.Token})
	if used.status != http.StatusOK {
		t.Fatalf("新会话应该能用，拿到 %d（%s）", used.status, used.body)
	}
	again := decode[authView](t, callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", `{"token":"`+env.AuthToken+`"}`).body)
	if again.Token == issued.Token || again.Token == "" {
		t.Fatalf("两次登录该签出不同的会话：%q / %q", issued.Token, again.Token)
	}

	wrong := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", `{"token":"nope"}`)
	if wrong.status != http.StatusUnauthorized || !strings.Contains(string(wrong.body), "token 不对") {
		t.Fatalf("token 不对应该 401：%d（%s）", wrong.status, wrong.body)
	}
	if token := decode[authView](t, wrong.body).Token; token != "" {
		t.Fatalf("验不过就不该给会话：%q", token)
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

// 认证步骤至少花掉 AuthFloor：成功与失败都一样，快慢里读不出东西。
func TestAuthStepHonoursFloor(t *testing.T) {
	const floor = 120 * time.Millisecond
	env := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{AuthFloor: floor})
	cases := []struct {
		name string
		body string
		want int
	}{
		{"token 对", `{"token":"` + testenv.DefaultAuthToken + `"}`, http.StatusOK},
		{"token 错", `{"token":"nope"}`, http.StatusUnauthorized},
		{"没带 token", `{}`, http.StatusUnauthorized},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			started := time.Now()
			response := callWith(t, env, env.RawClient, http.MethodPost, "/api/auth", testCase.body)
			elapsed := time.Since(started)
			if response.status != testCase.want {
				t.Fatalf("状态码 = %d，想要 %d（%s）", response.status, testCase.want, response.body)
			}
			if elapsed < floor {
				t.Fatalf("认证步骤只花了 %v，短于下限 %v", elapsed, floor)
			}
		})
	}
}

// 默认下限是 1 秒，调用方不指定也不会退化成"秒回"。
func TestAuthFloorDefault(t *testing.T) {
	if api.DefaultAuthFloor < time.Second {
		t.Fatalf("默认下限 = %v，至少要 1 秒", api.DefaultAuthFloor)
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
