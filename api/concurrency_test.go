package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/hm-service/scope-status/internal/testenv"
)

// 闸门各自独立：SSE 是长连接，一条连接要占着名额到客户端断开，所以它得自己一道门——
// 挂在业务闸门上，几十个看客就能把整个 /api 饿死（线上真出过：128 条 SSE 顶满闸门，
// 之后每个 /api 请求都排队到客户端放弃）。
func TestEventGateIsIndependent(t *testing.T) {
	env := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{MaxAPIConcurrency: 1, MaxEventConcurrency: 1})

	// holder 用带会话的 transport（不带就只会拿到 401，根本没进闸门），但不设超时：
	// 这条 SSE 一直占着事件闸门那唯一的槽，直到我们关掉它
	holder := &http.Client{Transport: env.Client.Transport}
	stream, err := holder.Get(env.BaseURL + "/api/events")
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("开 SSE 失败：%v（状态 %d）", err, stream.StatusCode)
	}
	defer stream.Body.Close()

	// 断言用短超时：万一这些端点也被那张闸门挡住，会立刻暴露，而不是等成慢速通过
	quick := &http.Client{Transport: env.Client.Transport, Timeout: 700 * time.Millisecond}

	// 业务端点：SSE 占满事件闸门，不该牵连业务闸门
	if got := callWith(t, env, quick, http.MethodGet, "/api/stats", ""); got.status != http.StatusOK {
		t.Fatalf("/api/stats = %d，SSE 不该占业务闸门", got.status)
	}
	// 第二条 SSE 会被事件闸门挡住：闸门满了就等着，等不到就放弃
	if _, err := quick.Get(env.BaseURL + "/api/events"); err == nil {
		t.Fatal("事件闸门已占满，第二条 SSE 不该还能进")
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
