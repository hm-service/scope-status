package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

func newMessageService(t *testing.T) (*MessageService, *Monitor) {
	t.Helper()
	messages := store.NewMessageStore(filepath.Join(t.TempDir(), "messages"), 1024)
	if err := messages.Ensure(); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	monitor := NewMonitor(tcpSource(t, listenTCP(t)), "", 0)
	monitor.SetSnapshotView(viewStub)
	return NewMessageService(messages, monitor), monitor
}

// 列表里每条消息都给完整内容，新的在前。
func TestPayloadGivesFullMessages(t *testing.T) {
	service, _ := newMessageService(t)
	first, err := service.Create("第一条", nil)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := service.Create("第二条", nil)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}

	payload, err := service.Payload()
	if err != nil {
		t.Fatalf("取列表失败：%v", err)
	}
	if payload.Summary.Total != 2 || len(payload.Messages) != 2 {
		t.Fatalf("概览不对：%+v / %d 条", payload.Summary, len(payload.Messages))
	}
	if payload.Messages[0].ID != second.ID || payload.Messages[0].Text != "第二条" {
		t.Fatalf("第一条视图不对：%+v", payload.Messages[0])
	}
	if payload.Messages[1].ID != first.ID || payload.Messages[1].Text != "第一条" {
		t.Fatalf("第二条视图不对：%+v", payload.Messages[1])
	}
}

func TestMutationsBroadcast(t *testing.T) {
	service, monitor := newMessageService(t)
	sub := monitor.Subscribe(i18n.Zh)
	defer monitor.Unsubscribe(sub)

	message, err := service.Create("你好", nil)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	events := sub.Drain()
	if len(events) != 1 || events[0].Name != "messages" {
		t.Fatalf("新建应该推一帧消息：%+v", events)
	}

	if _, err := service.Update(message.ID, ptr("改过了"), nil, false); err != nil {
		t.Fatalf("改消息失败：%v", err)
	}
	if _, err := service.Delete(message.ID); err != nil {
		t.Fatalf("删消息失败：%v", err)
	}
	if events := sub.Drain(); len(events) != 2 {
		t.Fatalf("改与删各该推一帧：%+v", events)
	}
	stats, err := service.Stats()
	if err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if stats.Live != 0 || stats.Deleted != 1 {
		t.Fatalf("统计不对：%+v", stats)
	}
}

func TestFailedMutationDoesNotBroadcast(t *testing.T) {
	service, monitor := newMessageService(t)
	sub := monitor.Subscribe(i18n.Zh)
	defer monitor.Unsubscribe(sub)
	if _, err := service.Create("   ", nil); err == nil {
		t.Fatal("空消息应该失败")
	}
	if events := sub.Drain(); len(events) != 0 {
		t.Fatalf("失败的变更不该推送：%+v", events)
	}
}

func TestUpdatedAtStamped(t *testing.T) {
	service, _ := newMessageService(t)
	message, err := service.Create("原文", nil)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	if message.UpdatedAt != nil {
		t.Fatalf("新建的消息不该有编辑时间：%v", *message.UpdatedAt)
	}
	before := time.Now().UnixMilli()
	updated, err := service.Update(message.ID, ptr("改过了"), nil, false)
	if err != nil {
		t.Fatalf("改消息失败：%v", err)
	}
	if updated.UpdatedAt == nil || *updated.UpdatedAt < before {
		t.Fatalf("编辑时间不对：%v", updated.UpdatedAt)
	}
}

func ptr(value string) *string { return &value }
