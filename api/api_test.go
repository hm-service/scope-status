package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/internal/testenv"
)

type reply struct {
	status int
	body   []byte
	header http.Header
}

// call 走测试实例自带的 client：每个请求都带 Authorization: Bearer。
func call(t *testing.T, env *testenv.Env, method, path, body string, headers ...[2]string) reply {
	t.Helper()
	return callWith(t, env, env.Client, method, path, body, headers...)
}

// callWith 指定 client：鉴权用例用 env.RawClient 造不带 token 的请求。
func callWith(t *testing.T, env *testenv.Env, client *http.Client, method, path, body string, headers ...[2]string) reply {
	t.Helper()
	request, err := http.NewRequest(method, env.BaseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("造请求失败：%v", err)
	}
	for _, header := range headers {
		request.Header.Set(header[0], header[1])
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s 失败：%v", method, path, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读响应失败：%v", err)
	}
	return reply{status: response.StatusCode, body: raw, header: response.Header}
}

func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("解析响应失败：%v（%s）", err, raw)
	}
	return value
}

func listenTCP(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了监听：%v", err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener.Addr().String()
}

func sourceWith(address string) string {
	return fmt.Sprintf(`
title: 测试看板
probe:
  method: tcp
  timeout: 0.5
peers:
  - name: 小明
    kind: friend
    note: 备注
    role: member
    addresses: ["%s"]
    properties:
      密码: secret
      端口: 51820
`, address)
}

type snapshotPayload struct {
	Title    string  `json:"title"`
	Method   string  `json:"method"`
	Reused   bool    `json:"reused"`
	Duration float64 `json:"durationMs"`
	Summary  struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
	} `json:"summary"`
	Groups []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Total  int    `json:"total"`
		Online int    `json:"online"`
	} `json:"groups"`
	Peers []struct {
		Name      string   `json:"name"`
		Note      string   `json:"note"`
		Kind      string   `json:"kind"`
		Core      bool     `json:"core"`
		Private   []string `json:"private"`
		Public    []string `json:"public"`
		WGPeers   []string `json:"wgPeers"`
		Online    bool     `json:"online"`
		LatencyMs *float64
		Addresses []struct {
			Address string  `json:"address"`
			Status  string  `json:"status"`
			Detail  *string `json:"detail"`
		} `json:"addresses"`
		Properties map[string]string `json:"properties"`
	} `json:"peers"`
	ServicesSummary struct {
		Total int `json:"total"`
	} `json:"servicesSummary"`
	LinksSummary struct {
		Total int `json:"total"`
	} `json:"linksSummary"`
}

func TestStatusSnapshot(t *testing.T) {
	address := listenTCP(t)
	env := testenv.New(t, sourceWith(address))

	response := call(t, env, http.MethodGet, "/api/status", "")
	if response.status != http.StatusOK {
		t.Fatalf("状态码 = %d（%s）", response.status, response.body)
	}
	if got := response.header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	payload := decode[snapshotPayload](t, response.body)
	if payload.Title != "测试看板" || payload.Method != "tcp" || payload.Reused {
		t.Fatalf("快照元信息不对：%+v", payload)
	}
	if payload.Summary.Total != 1 || payload.Summary.Online != 1 || payload.Summary.Offline != 0 {
		t.Fatalf("概览不对：%+v", payload.Summary)
	}
	if len(payload.Groups) != 1 || payload.Groups[0].Label != "朋友" || payload.Groups[0].Online != 1 {
		t.Fatalf("分组不对：%+v", payload.Groups)
	}
	peer := payload.Peers[0]
	if !peer.Online || peer.Note != "备注" || peer.Addresses[0].Status != "up" {
		t.Fatalf("条目不对：%+v", peer)
	}
	if peer.Kind != "friend" || peer.Core || peer.Private != nil || peer.WGPeers != nil {
		t.Fatalf("拓扑字段不对：kind=%q core=%v private=%v wg=%v", peer.Kind, peer.Core, peer.Private, peer.WGPeers)
	}
	if peer.Properties["密码"] != "secret" || peer.Properties["端口"] != "51820" {
		t.Fatalf("属性不对：%+v", peer.Properties)
	}

	forced := decode[snapshotPayload](t, call(t, env, http.MethodGet, "/api/status?force=1", "").body)
	if forced.Reused {
		t.Fatal("强制探测不该算复用")
	}
}

// 有 SSE 循环但还没有第一帧时，/api/status 回 204，让前端等推送。
func TestStatusWithoutSnapshotReturns204(t *testing.T) {
	env := testenv.NewWith(t, sourceWith(listenTCP(t)), testenv.Options{StreamInterval: time.Minute})
	response := call(t, env, http.MethodGet, "/api/status", "")
	if response.status != http.StatusNoContent || len(response.body) != 0 {
		t.Fatalf("状态码 = %d，body = %q", response.status, response.body)
	}
}

func TestReload(t *testing.T) {
	address := listenTCP(t)
	env := testenv.New(t, sourceWith(address))

	extra := strings.Replace(sourceWith(address), "peers:\n",
		"peers:\n  - name: 小红\n    kind: friend\n    addresses: [\"127.0.0.1:1\"]\n", 1)
	if err := os.WriteFile(env.SourceFile, []byte(extra), 0o600); err != nil {
		t.Fatalf("写配置失败：%v", err)
	}
	response := call(t, env, http.MethodPost, "/api/reload", "")
	if response.status != http.StatusOK {
		t.Fatalf("状态码 = %d（%s）", response.status, response.body)
	}
	payload := decode[struct {
		OK          bool   `json:"ok"`
		Message     string `json:"message"`
		Peers       int    `json:"peers"`
		ReloadCount int    `json:"reloadCount"`
		Snapshot    struct {
			Title string `json:"title"`
		} `json:"snapshot"`
	}](t, response.body)
	if !payload.OK || payload.Peers != 2 || payload.ReloadCount != 1 {
		t.Fatalf("重载结果不对：%+v", payload)
	}
	if !strings.Contains(payload.Message, "已重新加载 2 个条目") {
		t.Fatalf("重载提示 = %q", payload.Message)
	}
	if payload.Snapshot.Title != "测试看板" {
		t.Fatalf("重载后没探一次：%+v", payload.Snapshot)
	}

	if err := os.WriteFile(env.SourceFile, []byte("peers: 1\n"), 0o600); err != nil {
		t.Fatalf("写配置失败：%v", err)
	}
	broken := call(t, env, http.MethodPost, "/api/reload", "")
	if broken.status != http.StatusInternalServerError {
		t.Fatalf("写坏配置应该 500，拿到 %d", broken.status)
	}
	if !strings.Contains(string(broken.body), "peers 必须是数组") {
		t.Fatalf("应该把 YAML 的错误原文回给页面：%s", broken.body)
	}
	if got := len(env.App.Monitor.Source().Peers); got != 2 {
		t.Fatalf("写坏了要保留旧配置，现在有 %d 个条目", got)
	}
}

type messageView struct {
	ID          string  `json:"id"`
	Text        *string `json:"text"`
	UpdatedAt   *int64  `json:"updatedAt"`
	CreatedAt   int64   `json:"createdAt"`
	Attachments []struct {
		SHA256 string `json:"sha256"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		URL    string `json:"url"`
	} `json:"attachments"`
}

// 落盘的时间戳只到毫秒，同一毫秒建的消息排序才看随机后缀，所以按 id 找。
func findMessage(t *testing.T, payload messagesPayload, id string) messageView {
	t.Helper()
	for _, message := range payload.Messages {
		if message.ID == id {
			return message
		}
	}
	t.Fatalf("列表里找不到 %s", id)
	return messageView{}
}

type messagesPayload struct {
	Messages []messageView `json:"messages"`
	Summary  struct {
		Total int `json:"total"`
	} `json:"summary"`
}

func TestMessageLifecycle(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)

	created := call(t, env, http.MethodPost, "/api/messages", `{"text":"你好 world"}`)
	if created.status != http.StatusCreated {
		t.Fatalf("新建状态码 = %d（%s）", created.status, created.body)
	}
	message := decode[messageView](t, created.body)
	if message.Text == nil || *message.Text != "你好 world" {
		t.Fatalf("新建的视图不对：%+v", message)
	}
	if message.CreatedAt == 0 || message.UpdatedAt != nil {
		t.Fatalf("时间戳不对：%+v", message)
	}

	list := decode[messagesPayload](t, call(t, env, http.MethodGet, "/api/messages", "").body)
	if list.Summary.Total != 1 || len(list.Messages) != 1 {
		t.Fatalf("列表概览不对：%+v", list.Summary)
	}
	listed := findMessage(t, list, message.ID)
	if listed.Text == nil || *listed.Text != "你好 world" {
		t.Fatalf("列表里的视图不对：%+v", listed)
	}

	fetched := decode[messageView](t, call(t, env, http.MethodGet, "/api/messages/"+message.ID, "").body)
	if fetched.Text == nil || *fetched.Text != "你好 world" {
		t.Fatalf("取单条不对：%+v", fetched)
	}

	patched := decode[messageView](t, call(t, env, http.MethodPatch, "/api/messages/"+message.ID, `{"text":"改过了"}`).body)
	if patched.Text == nil || *patched.Text != "改过了" || patched.UpdatedAt == nil {
		t.Fatalf("编辑结果不对：%+v", patched)
	}

	deleted := decode[struct {
		OK           bool     `json:"ok"`
		ID           string   `json:"id"`
		RemovedFiles []string `json:"removedFiles"`
	}](t, call(t, env, http.MethodPost, "/api/messages/"+message.ID+"/delete", "").body)
	if !deleted.OK || deleted.ID != message.ID || len(deleted.RemovedFiles) != 0 {
		t.Fatalf("删除结果不对：%+v", deleted)
	}
	if gone := call(t, env, http.MethodGet, "/api/messages/"+message.ID, ""); gone.status != http.StatusNotFound {
		t.Fatalf("删掉的消息应该 404，拿到 %d", gone.status)
	}
}

func TestUploadAndDownload(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	content := "hello attachment"

	uploaded := call(t, env, http.MethodPut, "/api/messages/files?name=%E6%B5%8B%E8%AF%95.txt", content)
	if uploaded.status != http.StatusCreated {
		t.Fatalf("上传状态码 = %d（%s）", uploaded.status, uploaded.body)
	}
	attachment := decode[struct {
		SHA256 string `json:"sha256"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
	}](t, uploaded.body)
	if attachment.Name != "测试.txt" || attachment.Size != int64(len(content)) || len(attachment.SHA256) != 64 {
		t.Fatalf("上传结果不对：%+v", attachment)
	}

	created := decode[messageView](t, call(t, env, http.MethodPost, "/api/messages",
		fmt.Sprintf(`{"attachments":[{"sha256":"%s","name":"测试.txt"}]}`, attachment.SHA256)).body)
	if len(created.Attachments) != 1 || created.Attachments[0].URL == "" {
		t.Fatalf("新建带附件的消息不对：%+v", created)
	}

	download := call(t, env, http.MethodGet, "/api/messages/files/"+attachment.SHA256+"?id="+created.ID, "")
	if download.status != http.StatusOK || string(download.body) != content {
		t.Fatalf("下载不对：%d %q", download.status, download.body)
	}
	if got := download.header.Get("Content-Disposition"); !strings.Contains(got, "filename*=UTF-8''%E6%B5%8B%E8%AF%95.txt") {
		t.Fatalf("Content-Disposition = %q", got)
	}

	// 附件池是共享的：必须说明是哪个消息引用的它
	if missing := call(t, env, http.MethodGet, "/api/messages/files/"+attachment.SHA256, ""); missing.status != http.StatusBadRequest {
		t.Fatalf("不带 ?id= 应该 400，拿到 %d", missing.status)
	}

	deleted := decode[struct {
		RemovedFiles []string `json:"removedFiles"`
	}](t, call(t, env, http.MethodPost, "/api/messages/"+created.ID+"/delete", "").body)
	if len(deleted.RemovedFiles) != 1 || deleted.RemovedFiles[0] != attachment.SHA256 {
		t.Fatalf("删消息要回收没人引用的附件：%+v", deleted)
	}
	if gone := call(t, env, http.MethodGet, "/api/messages/files/"+attachment.SHA256+"?id="+created.ID, ""); gone.status != http.StatusNotFound {
		t.Fatalf("附件应该被回收了，拿到 %d", gone.status)
	}
}

func TestUploadLimits(t *testing.T) {
	env := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{MaxUploadBytes: 8})
	tooBig := call(t, env, http.MethodPut, "/api/messages/files?name=a.txt", strings.Repeat("x", 20))
	if tooBig.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限状态码 = %d（%s）", tooBig.status, tooBig.body)
	}
}

// cloud_servers 会并进 peers 列表；JSON 里给全 kind / core / private / public / wgPeers。
func TestCloudServersInPayload(t *testing.T) {
	env := testenv.New(t, `
title: 测试看板
probe:
  method: tcp
  timeout: 0.5
peers:
  - name: 小明
    kind: friend
    addresses: ["127.0.0.1:1"]
cloud_servers:
  - name: 云服务器-0
    core: true
    private: ["10.8.0.1"]
    public: ["203.0.113.10"]
  - name: 云服务器-1
    private: ["10.8.1.1"]
`)
	payload := decode[snapshotPayload](t, call(t, env, http.MethodGet, "/api/status", "").body)
	if len(payload.Peers) != 3 {
		t.Fatalf("peers = %d（云服务器应该并进来）", len(payload.Peers))
	}
	byName := map[string]int{}
	for index, peer := range payload.Peers {
		byName[peer.Name] = index
	}
	if index, ok := byName["云服务器-0"]; !ok {
		t.Fatalf("没有云服务器-0：%+v", byName)
	} else {
		cloud := payload.Peers[index]
		if cloud.Kind != "cloud" || !cloud.Core {
			t.Fatalf("kind/core = %q/%v", cloud.Kind, cloud.Core)
		}
		if len(cloud.Private) != 1 || cloud.Private[0] != "10.8.0.1" || len(cloud.Public) != 1 || cloud.Public[0] != "203.0.113.10" {
			t.Fatalf("private/public = %v/%v", cloud.Private, cloud.Public)
		}
		if len(cloud.Addresses) != 2 {
			t.Fatalf("探测地址 = %+v", cloud.Addresses)
		}
		// WG 链路按 core 推：主干列出其余每一台
		if len(cloud.WGPeers) != 1 || cloud.WGPeers[0] != "云服务器-1" {
			t.Fatalf("主干的 wgPeers = %v", cloud.WGPeers)
		}
	}
	if index := byName["云服务器-1"]; len(payload.Peers[index].WGPeers) != 1 || payload.Peers[index].WGPeers[0] != "云服务器-0" {
		t.Fatalf("非主干的 wgPeers = %v", payload.Peers[index].WGPeers)
	}
	if index := byName["小明"]; payload.Peers[index].WGPeers != nil || payload.Peers[index].Core {
		t.Fatalf("普通条目不该有 core/wgPeers：%+v", payload.Peers[index])
	}
}

func TestErrorTextFollowsLanguage(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	english := call(t, env, http.MethodGet, "/api/messages/not-an-id", "", [2]string{"Accept-Language", "en-US,en;q=0.9"})
	if !strings.Contains(string(english.body), "bad message id") {
		t.Fatalf("英文文案 = %q", english.body)
	}
	chinese := call(t, env, http.MethodGet, "/api/messages/not-an-id", "", [2]string{"Accept-Language", "zh-CN"})
	if !strings.Contains(string(chinese.body), "消息 id 不合法") {
		t.Fatalf("中文文案 = %q", chinese.body)
	}
	if english.status != http.StatusBadRequest || chinese.status != http.StatusBadRequest {
		t.Fatalf("状态码不对：%d / %d", english.status, chinese.status)
	}
}

func TestEventsStream(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
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
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	reader := bufio.NewReader(response.Body)
	first := readEvent(t, reader)
	if first.name != "messages" {
		t.Fatalf("第一帧应该是消息列表，拿到 %q", first.name)
	}
	second := readEvent(t, reader)
	if second.name != "snapshot" && second.name != "hello" {
		t.Fatalf("第二帧应该是快照或 hello，拿到 %q", second.name)
	}
	if second.name == "snapshot" && !strings.Contains(second.data, `"summary"`) {
		t.Fatalf("快照帧内容不对：%s", second.data)
	}

	// 消息有变更要主动推一帧
	go func() {
		time.Sleep(50 * time.Millisecond)
		call(t, env, http.MethodPost, "/api/messages", `{"text":"推送测试"}`)
	}()
	third := readEvent(t, reader)
	if third.name != "messages" || !strings.Contains(third.data, "推送测试") {
		t.Fatalf("消息变更应该推一帧：%q %s", third.name, third.data)
	}
}

type errorBody struct {
	Detail string `json:"detail"`
}

func TestErrorBodyAndHeaders(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)

	bad := call(t, env, http.MethodGet, "/api/messages/not-an-id", "")
	if bad.status != http.StatusBadRequest {
		t.Fatalf("状态码 = %d", bad.status)
	}
	if got := bad.header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("错误体应该是 JSON：%q", got)
	}
	if detail := decode[errorBody](t, bad.body).Detail; detail == "" {
		t.Fatalf("错误体缺少 detail：%s", bad.body)
	}
	if got := bad.header.Get("Content-Language"); got != "zh" {
		t.Fatalf("Content-Language = %q", got)
	}
	if got := bad.header.Get("Vary"); !strings.Contains(got, "Accept-Language") {
		t.Fatalf("Vary = %q", got)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
	} {
		if got := bad.header.Get(header); got != want {
			t.Fatalf("%s = %q，想要 %q", header, got, want)
		}
	}

	wrong := call(t, env, http.MethodDelete, "/api/health", "")
	if wrong.status != http.StatusMethodNotAllowed {
		t.Fatalf("方法不对应该 405，拿到 %d（%s）", wrong.status, wrong.body)
	}
	if got := wrong.header.Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q", got)
	}
	if decode[errorBody](t, wrong.body).Detail == "" {
		t.Fatalf("405 也要有 detail：%s", wrong.body)
	}

	// /api/messages/files 只接受 PUT，但它和 GET /api/messages/{message_id} 不可比，
	// 所以 405 得按实际路径把 Allow 查回来。
	upload := call(t, env, http.MethodPost, "/api/messages/files?name=a.txt", "x")
	if upload.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST 到上传口应该 405，拿到 %d（%s）", upload.status, upload.body)
	}
	if got := upload.header.Get("Allow"); got != "PUT" {
		t.Fatalf("上传口的 Allow = %q，想要 PUT", got)
	}
}

// readyz 失败时细节只进后端日志，不回给调用方。
func TestReadyzKeepsDetailInLog(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	if err := os.Chmod(env.DataDir, 0o500); err != nil {
		t.Fatalf("改权限失败：%v", err)
	}
	t.Cleanup(func() { os.Chmod(env.DataDir, 0o700) })

	response := call(t, env, http.MethodGet, "/readyz", "")
	if response.status != http.StatusServiceUnavailable {
		t.Fatalf("数据目录权限不对应该 503，拿到 %d（%s）", response.status, response.body)
	}
	if strings.Contains(string(response.body), env.DataDir) {
		t.Fatalf("不应该把细节回给调用方：%s", response.body)
	}
}

type event struct {
	name string
	data string
}

func readEvent(t *testing.T, reader *bufio.Reader) event {
	t.Helper()
	var current event
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("读 SSE 失败：%v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			current.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		case line == "" && current.name != "":
			return current
		}
	}
}

func TestHealthAndProbes(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	health := decode[struct {
		Status         string `json:"status"`
		Peers          int    `json:"peers"`
		Categories     []string
		StreamInterval float64 `json:"streamInterval"`
		Messages       struct {
			Live int `json:"live"`
		} `json:"messages"`
		MessagesDir    string `json:"messagesDir"`
		MaxUploadBytes int64  `json:"maxUploadBytes"`
	}](t, call(t, env, http.MethodGet, "/api/health", "").body)
	if health.Status != "ok" || health.Peers != 1 || health.MaxUploadBytes != 1024*1024 {
		t.Fatalf("健康信息不对：%+v", health)
	}
	if !strings.HasSuffix(health.MessagesDir, filepath.Join("data", "messages")) {
		t.Fatalf("消息目录 = %q", health.MessagesDir)
	}
	if health.Categories[0] != "friend" {
		t.Fatalf("分类 = %v", health.Categories)
	}
	if response := call(t, env, http.MethodGet, "/livez", ""); response.status != http.StatusOK {
		t.Fatalf("/livez = %d", response.status)
	}
	if response := call(t, env, http.MethodGet, "/readyz", ""); response.status != http.StatusOK {
		t.Fatalf("/readyz = %d（%s）", response.status, response.body)
	}

	guarded := testenv.NewWith(t, testenv.DefaultSource, testenv.Options{ReadyzToken: "tok_0123456789ab"})
	if response := call(t, guarded, http.MethodGet, "/readyz", ""); response.status != http.StatusUnauthorized {
		t.Fatalf("没带 token 应该 401，拿到 %d", response.status)
	}
	if response := call(t, guarded, http.MethodGet, "/readyz?token=wrong", ""); response.status != http.StatusUnauthorized {
		t.Fatalf("token 不对应该 401，拿到 %d", response.status)
	}
	if response := call(t, guarded, http.MethodGet, "/readyz?token=tok_0123456789ab", ""); response.status != http.StatusOK {
		t.Fatalf("token 对了应该 200，拿到 %d", response.status)
	}
}

func TestStaticFiles(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	index := call(t, env, http.MethodGet, "/", "")
	if index.status != http.StatusOK || !strings.Contains(string(index.body), "<title>test</title>") {
		t.Fatalf("首页不对：%d %q", index.status, index.body)
	}
	asset := call(t, env, http.MethodGet, "/assets/app.js", "")
	if asset.status != http.StatusOK || asset.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("静态资源不对：%d %v", asset.status, asset.header)
	}
	if missing := call(t, env, http.MethodGet, "/nope.html", ""); missing.status != http.StatusNotFound {
		t.Fatalf("不存在的文件应该 404，拿到 %d", missing.status)
	}
	method := call(t, env, http.MethodPost, "/", "")
	if method.status != http.StatusMethodNotAllowed || method.header.Get("Allow") != "GET, HEAD" {
		t.Fatalf("静态路径只允许 GET/HEAD：%d %v", method.status, method.header)
	}
	unknown := call(t, env, http.MethodGet, "/api/nope", "")
	if unknown.status != http.StatusNotFound {
		t.Fatalf("未知 API 应该回 404：%d %s", unknown.status, unknown.body)
	}
	if detail := decode[errorBody](t, unknown.body).Detail; detail == "" {
		t.Fatalf("错误体应该是 {detail: …}：%s", unknown.body)
	}
}
