package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/hm-service/scope-status/internal/testenv"
)

// 四道闸门各自独立：把业务那道占满，探针、认证还得照常走。
// 旧实现是一张闸门统管 /api/* + /livez + /readyz，一条 SSE 就能把探针一起饿死。
func TestConcurrencyGatesAreIndependent(t *testing.T) {
	env := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{MaxAPIConcurrency: 1, MaxAuthConcurrency: 1})

	// holder 用带会话的 transport（不带就只会拿到 401，根本没进闸门），但不设超时：
	// SSE 一直占着业务闸门那唯一的槽，直到我们关掉它
	holder := &http.Client{Transport: env.Client.Transport}
	stream, err := holder.Get(env.BaseURL + "/api/events")
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("开 SSE 失败：%v（状态 %d）", err, stream.StatusCode)
	}
	defer stream.Body.Close()

	// 断言用短超时：万一这些端点也被那张闸门挡住，会立刻暴露，而不是等成慢速通过
	quick := &http.Client{Transport: env.Client.Transport, Timeout: 700 * time.Millisecond}

	// 业务端点：闸门占满了就该等着，等不到就放弃
	if _, err := quick.Get(env.BaseURL + "/api/health"); err == nil {
		t.Fatal("业务闸门已占满，/api/health 不该还能进")
	}
	// /livez 不设闸：活着就要答得上
	if got := callWith(t, env, quick, http.MethodGet, "/livez", ""); got.status != http.StatusOK {
		t.Fatalf("/livez = %d，没有闸门就该是 200", got.status)
	}
	// /readyz 与 /api/auth 各走自己的闸门
	if got := callWith(t, env, quick, http.MethodGet, "/readyz", ""); got.status != http.StatusOK {
		t.Fatalf("/readyz = %d，该走自己的闸门拿到 200", got.status)
	}
	if got := callWith(t, env, quick, http.MethodPost, "/api/auth", `{"token":"wrong_token_0123456"}`); got.status != http.StatusUnauthorized {
		t.Fatalf("/api/auth = %d，该走自己的闸门拿到 401", got.status)
	}
}
