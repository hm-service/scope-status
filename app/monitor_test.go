package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func parseSource(t *testing.T, body string) *core.Source {
	t.Helper()
	source, err := core.ParseSource("test.yaml", []byte(body))
	if err != nil {
		t.Fatalf("配置解析失败：%v", err)
	}
	return source
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

func closedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了监听：%v", err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func tcpSource(t *testing.T, addresses ...string) *core.Source {
	t.Helper()
	body := "title: 测试\nprobe:\n  method: tcp\n  timeout: 0.5\npeers:\n"
	for index, address := range addresses {
		body += fmt.Sprintf("  - name: 机器%d\n    kind: friend\n    addresses: [\"%s\"]\n", index, address)
	}
	return parseSource(t, body)
}

func viewStub(snapshot *core.Snapshot, lang i18n.Lang, reused bool) any {
	return map[string]any{"title": snapshot.TitleText(lang), "reused": reused}
}

func TestRefreshBuildsSnapshot(t *testing.T) {
	live := listenTCP(t)
	dead := closedAddress(t)
	monitor := NewMonitor(tcpSource(t, live, dead), "", 0)

	snapshot, err := monitor.Refresh()
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if snapshot.Summary.Total != 2 || snapshot.Summary.Online != 1 || snapshot.Summary.Offline != 1 {
		t.Fatalf("概览不对：%+v", snapshot.Summary)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].Category.ID != "friend" || snapshot.Groups[0].Online != 1 {
		t.Fatalf("分组不对：%+v", snapshot.Groups)
	}
	if !snapshot.Peers[0].Online || snapshot.Peers[0].LatencyMs == nil {
		t.Fatalf("第一台应该通：%+v", snapshot.Peers[0])
	}
	if snapshot.Peers[1].Online || snapshot.Peers[1].Addresses[0].Status != core.StatusDown {
		t.Fatalf("第二台应该不通：%+v", snapshot.Peers[1])
	}
	if snapshot.Method != core.MethodTCP || snapshot.GeneratedAt.IsZero() {
		t.Fatalf("快照元信息不对：%+v", snapshot)
	}
}

func TestProbeFailureSurfaces(t *testing.T) {
	source := parseSource(t, "probe:\n  method: tcp\n  timeout: 0.3\npeers:\n  - name: 小明\n    kind: friend\n    addresses: [\"nope.invalid:1234\"]\n")
	monitor := NewMonitor(source, "", 0)
	_, err := monitor.Refresh()
	if err == nil {
		t.Fatal("解析不了的地址应该报错，不能编一个状态")
	}
	var business *core.Error
	if !errors.As(err, &business) || business.Key != i18n.KeyProbeDNS {
		t.Fatalf("错误不对：%v", err)
	}
}

func TestSnapshotReuseDependsOnStreamLoop(t *testing.T) {
	live := listenTCP(t)
	source := tcpSource(t, live)

	looping := NewMonitor(source, "", time.Minute)
	if snapshot, reused, err := looping.Snapshot(false); snapshot != nil || reused || err != nil {
		t.Fatalf("还没有第一帧时应该回 204 语义：%+v reused=%v err=%v", snapshot, reused, err)
	}
	if _, err := looping.Refresh(); err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if _, reused, err := looping.Snapshot(false); err != nil || !reused {
		t.Fatalf("有循环时应该复用快照：reused=%v err=%v", reused, err)
	}
	if _, reused, err := looping.Snapshot(true); err != nil || reused {
		t.Fatalf("强制探测不该算复用：reused=%v err=%v", reused, err)
	}

	onDemand := NewMonitor(source, "", 0)
	if _, _, err := onDemand.Snapshot(false); err != nil {
		t.Fatalf("没有循环时按需探测：%v", err)
	}
	if _, reused, err := onDemand.Snapshot(false); err != nil || reused {
		t.Fatalf("没有循环时每次都要真探：reused=%v err=%v", reused, err)
	}
}

func TestBroadcastToSubscribers(t *testing.T) {
	monitor := NewMonitor(tcpSource(t, listenTCP(t)), "", 0)
	monitor.SetSnapshotView(viewStub)
	sub := monitor.Subscribe(i18n.Zh)
	defer monitor.Unsubscribe(sub)

	if _, err := monitor.Refresh(); err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	events := sub.Drain()
	if len(events) != 1 || events[0].Name != "snapshot" {
		t.Fatalf("订阅者应该收到一帧快照：%+v", events)
	}
	rendered, ok := events[0].Payload.Render(i18n.En).(map[string]any)
	if !ok || rendered["reused"] != false {
		t.Fatalf("负载没按语言渲染：%+v", rendered)
	}
	if monitor.Subscribers() != 1 {
		t.Fatalf("订阅者数 = %d", monitor.Subscribers())
	}
	monitor.Unsubscribe(sub)
	if monitor.Subscribers() != 0 {
		t.Fatalf("退订后订阅者数 = %d", monitor.Subscribers())
	}
}

// 队列满了丢最旧的：订阅者掉队不该拖住服务端。
func TestSubscriberQueueDropsOldest(t *testing.T) {
	monitor := NewMonitor(tcpSource(t), "", 0)
	sub := monitor.Subscribe(i18n.Zh)
	defer monitor.Unsubscribe(sub)
	for index := 0; index < subscriberQueue+3; index++ {
		monitor.Broadcast("messages", RawPayload{Value: index})
	}
	events := sub.Drain()
	if len(events) != subscriberQueue {
		t.Fatalf("队列长度 = %d，想要 %d", len(events), subscriberQueue)
	}
	if events[0].Payload.Render(i18n.Zh) != 3 {
		t.Fatalf("最旧的帧没被丢掉：%+v", events[0].Payload.Render(i18n.Zh))
	}
}

func TestStreamLoopStopsOnProbeFailure(t *testing.T) {
	source := parseSource(t, "probe:\n  method: tcp\n  timeout: 0.2\npeers:\n  - name: 小明\n    kind: friend\n    addresses: [\"nope.invalid:1\"]\n")
	monitor := NewMonitor(source, "", 10*time.Millisecond)
	done := make(chan struct{})
	go func() {
		monitor.Stream(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("探测失败后循环应该停下来")
	}
	if monitor.Current() != nil {
		t.Fatal("失败的那轮不该留下快照")
	}
}

func TestStreamLoopStopsOnCancel(t *testing.T) {
	monitor := NewMonitor(tcpSource(t, listenTCP(t)), "", 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		monitor.Stream(ctx)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for monitor.Current() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if monitor.Current() == nil {
		t.Fatal("循环应该探出第一帧")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("取消后循环应该退出")
	}
}

func TestReload(t *testing.T) {
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "source.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(sourceFile, []byte(body), 0o600); err != nil {
			t.Fatalf("写配置失败：%v", err)
		}
	}
	write("peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"127.0.0.1:1\"]\n")
	source, err := core.ParseSource(sourceFile, []byte("peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"127.0.0.1:1\"]\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	monitor := NewMonitor(source, sourceFile, 0)
	monitor.SetSnapshotView(viewStub)
	if _, err := monitor.Refresh(); err != nil {
		t.Fatalf("探测失败：%v", err)
	}

	write("peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"127.0.0.1:1\"]\n  - name: 小红\n    kind: friend\n    addresses: [\"127.0.0.1:2\"]\n")
	reloaded, err := monitor.Reload()
	if err != nil {
		t.Fatalf("重载失败：%v", err)
	}
	if len(reloaded.Peers) != 2 || monitor.ReloadCount() != 1 {
		t.Fatalf("重载结果不对：%d / %d", len(reloaded.Peers), monitor.ReloadCount())
	}
	if monitor.Current() != nil {
		t.Fatal("换了配置要丢掉旧快照")
	}

	write("peers: 1\n")
	if _, err := monitor.Reload(); err == nil {
		t.Fatal("配置写坏了应该报错")
	}
	if got := len(monitor.Source().Peers); got != 2 {
		t.Fatalf("写坏了要保留旧配置，现在有 %d 个条目", got)
	}
}

func TestMonitorIgnoresNegativeStreamInterval(t *testing.T) {
	monitor := NewMonitor(tcpSource(t), "", -5*time.Second)
	if monitor.StreamInterval() != 0 {
		t.Fatalf("间隔 = %v", monitor.StreamInterval())
	}
}

// 回归：空闲时循环会睡一分钟，订阅者连上必须立刻把它叫醒——
// 否则首帧探得快的时候（比如图标改成异步之后），新打开的页面要僵一分钟。
func TestStreamWakesUpForNewSubscriber(t *testing.T) {
	monitor := NewMonitor(tcpSource(t, listenTCP(t)), "", 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		monitor.Stream(ctx)
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for monitor.Current() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	first := monitor.Current()
	if first == nil {
		t.Fatal("第一帧没出来")
	}
	// 没有订阅者：循环进入每分钟一次的空闲节奏
	time.Sleep(300 * time.Millisecond)
	if monitor.Current() != first {
		t.Fatal("空闲时不该再探")
	}
	sub := monitor.Subscribe(i18n.Zh)
	defer monitor.Unsubscribe(sub)
	if !waitForSnapshot(t, monitor, first, 2*time.Second) {
		t.Fatal("订阅者连上后没有被叫醒，快照还是旧的")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("取消后循环没退出")
	}
}

func waitForSnapshot(t *testing.T, monitor *Monitor, previous *core.Snapshot, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if current := monitor.Current(); current != nil && current != previous {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
