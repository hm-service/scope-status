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

// PayloadSince 只给「最近一次活动」晚于 since 的消息，概览的总数跟着变，LatestAt 不受筛选影响。
func TestPayloadSinceFiltersByActivity(t *testing.T) {
	service, _ := newMessageService(t)
	store := service.Store()
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)

	// 时间要可控：直接写存储，不靠 sleep
	older, err := store.Create("旧的", nil, base)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	if _, err := store.Create("新的", nil, base.Add(time.Minute)); err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	// 编辑旧的那条：它的活动时间因此变成最新的
	if _, err := store.Update(older.ID, ptr("旧的（刚改）"), nil, false, base.Add(2*time.Minute).UnixMilli()); err != nil {
		t.Fatalf("改消息失败：%v", err)
	}
	newest := base.Add(2 * time.Minute).UnixMilli()

	all, err := service.PayloadSince(0)
	if err != nil {
		t.Fatalf("取列表失败：%v", err)
	}
	if all.Summary.Total != 2 || len(all.Messages) != 2 {
		t.Fatalf("since=0 应该给全部：%+v / %d 条", all.Summary, len(all.Messages))
	}
	if all.Summary.LatestAt != newest {
		t.Fatalf("LatestAt = %d，想要 %d", all.Summary.LatestAt, newest)
	}

	cases := []struct {
		name      string
		since     time.Time
		wantTotal int
		wantIDs   []string
	}{
		{"比创建时间新：两条都在", base.Add(30 * time.Second), 2, nil},
		{"只有改过的那条更新", base.Add(90 * time.Second), 1, []string{older.ID}},
		{"比谁都新：一条都没有", base.Add(3 * time.Minute), 0, []string{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := service.PayloadSince(testCase.since.UnixMilli())
			if err != nil {
				t.Fatalf("取列表失败：%v", err)
			}
			if payload.Summary.Total != testCase.wantTotal || len(payload.Messages) != testCase.wantTotal {
				t.Fatalf("条数 = %d/%d，想要 %d", payload.Summary.Total, len(payload.Messages), testCase.wantTotal)
			}
			if payload.Summary.LatestAt != newest {
				t.Fatalf("LatestAt = %d，想要 %d（不该被筛选影响）", payload.Summary.LatestAt, newest)
			}
			for index, id := range testCase.wantIDs {
				if payload.Messages[index].ID != id {
					t.Fatalf("第 %d 条 = %s，想要 %s", index, payload.Messages[index].ID, id)
				}
			}
		})
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
