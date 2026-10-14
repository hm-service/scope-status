package api_test

import (
	"net/http"
	"testing"

	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/internal/testenv"
)

// 安全头挂在最外层中间件上：静态页、API、探针，连 401 这种错误响应也要带 CSP。
func TestSecurityHeadersOnTheWire(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	cases := []struct {
		name string
		path string
		want int
	}{
		{"静态首页", "/", http.StatusOK},
		{"探针", "/livez", http.StatusOK},
		{"未鉴权 API", "/api/health", http.StatusUnauthorized},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := callWith(t, env, env.RawClient, http.MethodGet, testCase.path, "")
			if got.status != testCase.want {
				t.Fatalf("%s 状态 = %d，想要 %d", testCase.path, got.status, testCase.want)
			}
			if csp := got.header.Get("Content-Security-Policy"); csp != httpx.ContentSecurityPolicy {
				t.Fatalf("%s 的 CSP = %q，想要 %q", testCase.path, csp, httpx.ContentSecurityPolicy)
			}
		})
	}
}
