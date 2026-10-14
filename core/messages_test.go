package core

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

func TestMessageIDRoundTrip(t *testing.T) {
	moment := time.Date(2026, 10, 12, 23, 30, 54, 852*int(time.Millisecond), time.Local)
	id, err := NewMessageID(moment, func(string) bool { return false })
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	if !regexp.MustCompile(`^message-2026-10-12-23-30-54-0852-[a-z0-9]{6}$`).MatchString(id) {
		t.Fatalf("id 形状不对：%q", id)
	}
	created, err := ParseMessageID(id)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if !created.Equal(moment) {
		t.Fatalf("往返不一致：%v != %v", created, moment)
	}
}

// id 里的时间来自文件名：撞号时要换后缀，不能换时间。
func TestNewMessageIDRetriesOnCollision(t *testing.T) {
	moment := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	taken := map[string]bool{}
	id, err := NewMessageID(moment, func(candidate string) bool { return taken[candidate] })
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	second, err := NewMessageID(moment, func(candidate string) bool { return candidate == id })
	if err != nil {
		t.Fatalf("生成失败：%v", err)
	}
	if second == id {
		t.Fatalf("撞号了没换后缀")
	}
	if !strings.HasPrefix(second, "message-2026-01-02-03-04-05-0000-") {
		t.Fatalf("换了时间戳：%q", second)
	}
}

func TestParseMessageIDRejectsBad(t *testing.T) {
	cases := []string{
		"",
		"message-2026-10-12",
		"message-2026-13-12-23-30-54-0852-abc123",
		"message-2026-02-30-23-30-54-0852-abc123",
		"message-2026-10-12-23-30-54-0852-ABC123",
		"message-2026-10-12-23-30-54-0852-abc12",
		"2026-10-12-23-30-54-0852-abc123",
	}
	for _, id := range cases {
		if _, err := ParseMessageID(id); err == nil {
			t.Errorf("%q 应该被拒绝", id)
		}
	}
}

func TestCleanFileName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"普通名字", "报告.pdf", "报告.pdf"},
		{"去掉路径", "/etc/passwd", "passwd"},
		{"windows 路径", `C:\tmp\a.txt`, "a.txt"},
		{"去掉控制字符", "a\x00b.txt", "ab.txt"},
		{"首尾空格", "  a.txt  ", "a.txt"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := CleanFileName(testCase.input)
			if err != nil {
				t.Fatalf("不该报错：%v", err)
			}
			if got != testCase.want {
				t.Fatalf("CleanFileName(%q) = %q，想要 %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestCleanFileNameRejects(t *testing.T) {
	cases := []struct {
		input string
		key   i18n.Key
	}{
		{"", i18n.KeyMessageNameMissing},
		{"   ", i18n.KeyMessageNameMissing},
		{"..", i18n.KeyMessageNameInvalid},
		{"/", i18n.KeyMessageNameInvalid},
		{"\x01", i18n.KeyMessageNameInvalid},
	}
	for _, testCase := range cases {
		_, err := CleanFileName(testCase.input)
		if err == nil {
			t.Fatalf("%q 应该被拒绝", testCase.input)
		}
		if key := errorKey(t, err); key != testCase.key {
			t.Fatalf("%q 的错误键 = %q，想要 %q", testCase.input, key, testCase.key)
		}
	}
}

// 未读按「最近一次活动」算：编辑过就算编辑时间，没编辑过就是创建时间。
func TestMessageActivity(t *testing.T) {
	created := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
	stamp := func(value int64) *int64 { return &value }

	cases := []struct {
		name      string
		updatedAt *int64
		want      int64
	}{
		{"没编辑过就是创建时间", nil, created.UnixMilli()},
		{"编辑过就用编辑时间", stamp(created.UnixMilli() + 5000), created.UnixMilli() + 5000},
		{"编辑时间与创建时间相同", stamp(created.UnixMilli()), created.UnixMilli()},
		{"编辑时间早于创建时间（不该发生）仍按创建时间", stamp(created.UnixMilli() - 1), created.UnixMilli()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			message := Message{CreatedAt: created, UpdatedAt: testCase.updatedAt}
			if got := message.Activity(); got != testCase.want {
				t.Fatalf("Activity = %d，想要 %d", got, testCase.want)
			}
		})
	}
}

func TestMessageViews(t *testing.T) {
	message := &Message{
		ID:        "message-2026-10-12-23-30-54-0852-abc123",
		Text:      "正文",
		CreatedAt: time.Date(2026, 10, 12, 23, 30, 54, 852*int(time.Millisecond), time.Local),
		Attachments: []Attachment{
			{SHA256: strings.Repeat("a", 64), Name: "a.txt", Size: 3},
		},
	}
	view := message.View()
	if view.Text != "正文" || view.ID != message.ID || view.CreatedAt != message.CreatedAt.UnixMilli() || len(view.Attachments) != 1 {
		t.Fatalf("视图不对：%+v", view)
	}
	if !strings.Contains(view.Attachments[0].URL, "?id="+message.ID) {
		t.Fatalf("附件地址没带消息 id：%q", view.Attachments[0].URL)
	}
}
