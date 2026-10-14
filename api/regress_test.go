package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/internal/testenv"
)

// 回归：上传不带 ?name= 时要 400，不能把空名字当成一个附件存下来。
func TestRegressUploadWithoutName(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	response := call(t, env, http.MethodPut, "/api/messages/files", "content")
	if response.status != http.StatusBadRequest {
		t.Fatalf("状态码 = %d（%s）", response.status, response.body)
	}
	if !strings.Contains(string(response.body), "缺少附件名") {
		t.Fatalf("错误文案 = %q", response.body)
	}
}

// 回归：还没有快照时的 hello 帧只带推送间隔，不要塞别的字段。
func TestRegressHelloFrameShape(t *testing.T) {
	env := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{StreamInterval: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, env.BaseURL+"/api/events", nil)
	if err != nil {
		t.Fatalf("造请求失败：%v", err)
	}
	response, err := env.Client.Do(request)
	if err != nil {
		t.Fatalf("连 SSE 失败：%v", err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if first := readEvent(t, reader); first.name != "messages" {
		t.Fatalf("第一帧 = %q", first.name)
	}
	hello := readEvent(t, reader)
	if hello.name != "hello" {
		t.Fatalf("第二帧 = %q", hello.name)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(hello.data), &fields); err != nil {
		t.Fatalf("hello 帧不是 JSON：%v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("hello 帧只该有 streamInterval：%v", fields)
	}
	if fields["streamInterval"] != float64(60) {
		t.Fatalf("streamInterval = %v", fields["streamInterval"])
	}
}

// 回归：/api/messages/files 与 /api/messages/{message_id} 在 ServeMux 里互不可比（直接 panic），
// 现在改成手写分段路由：字面量优先，Allow 按真实方法给。
func TestRegressAmbiguousRoutes(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)

	get := call(t, env, http.MethodGet, "/api/messages/files", "")
	if get.status != http.StatusMethodNotAllowed || get.header.Get("Allow") != "PUT" {
		t.Fatalf("GET 上传口应该是 405 + Allow: PUT，拿到 %d %q", get.status, get.header.Get("Allow"))
	}
	patch := call(t, env, http.MethodPatch, "/api/messages/files", "{}")
	if patch.status != http.StatusMethodNotAllowed || patch.header.Get("Allow") != "PUT" {
		t.Fatalf("PATCH 上传口应该是 405 + Allow: PUT，拿到 %d %q", patch.status, patch.header.Get("Allow"))
	}
	// 下载路由仍然可达（缺 ?id= 时 400），说明字面量优先没有把通配路由吃掉
	download := call(t, env, http.MethodGet, "/api/messages/files/"+strings.Repeat("a", 64), "")
	if download.status != http.StatusBadRequest {
		t.Fatalf("下载口缺 ?id= 应该 400，拿到 %d（%s）", download.status, download.body)
	}
	// 通配路由的方法兜底也要准
	remove := call(t, env, http.MethodGet, "/api/messages/message-2026-01-01-00-00-00-0000-abc123/delete", "")
	if remove.status != http.StatusMethodNotAllowed || remove.header.Get("Allow") != "POST" {
		t.Fatalf("delete 的 405 不对：%d %q", remove.status, remove.header.Get("Allow"))
	}
}
