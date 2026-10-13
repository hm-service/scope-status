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

func TestAuthorize(t *testing.T) {
	hashed, err := HashKey("s3cret")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	key := &Message{ID: "message-2026-10-12-23-30-54-0852-abc123", Kind: KindKey, KeyHash: hashed}
	public := &Message{ID: "message-2026-10-12-23-30-54-0852-abc123", Kind: KindPublic}

	if err := Authorize(public, "", 0); err != nil {
		t.Fatalf("普通消息不该要口令：%v", err)
	}
	if err := Authorize(key, "s3cret", 0); err != nil {
		t.Fatalf("口令对了还报错：%v", err)
	}
	if got := errorKey(t, Authorize(key, "", 0)); got != i18n.KeyMessageKeyRequired {
		t.Fatalf("缺口令的错误键 = %q", got)
	}
	if got := errorKey(t, Authorize(key, "wrong", 0)); got != i18n.KeyMessageKeyWrong {
		t.Fatalf("口令错的错误键 = %q", got)
	}
	broken := &Message{ID: key.ID, Kind: KindKey, KeyHash: "明文不是哈希"}
	if got := errorKey(t, Authorize(broken, "s3cret", 0)); got != i18n.KeyMessageFileKey {
		t.Fatalf("坏哈希的错误键 = %q", got)
	}
}

func TestHashKeyRoundTrip(t *testing.T) {
	hashed, err := HashKey("秘密的口令")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	if !strings.HasPrefix(hashed, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("PHC 串形状不对：%q", hashed)
	}
	if strings.Contains(hashed, "秘密的口令") {
		t.Fatal("哈希串里不该出现明文")
	}
	if !ValidKeyHash(hashed) {
		t.Fatal("自己生成的哈希应该能解析")
	}
	matched, err := VerifyKey("秘密的口令", hashed, 0)
	if err != nil || !matched {
		t.Fatalf("口令对了没验过：%v / %v", matched, err)
	}
	matched, err = VerifyKey("别的口令", hashed, 0)
	if err != nil || matched {
		t.Fatalf("口令错了却验过：%v / %v", matched, err)
	}
	// 同一个口令两次哈希必须不同（盐随机）
	again, err := HashKey("秘密的口令")
	if err != nil || again == hashed {
		t.Fatalf("盐没随机：%v", err)
	}
}

func TestVerifyKeyRejectsBrokenHash(t *testing.T) {
	cases := map[string]string{
		"不是 PHC": "s3cret",
		"算法不对":   "$argon2i$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"版本不对":   "$argon2id$v=13$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"参数被放大":  "$argon2id$v=19$m=999999999,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"盐太短":    "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaGhhc2hoYXNoaGFzaA",
		"摘要太短":   "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaA",
		"字段数不对":  "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if ValidKeyHash(encoded) {
				t.Fatalf("%q 不该被当成合法哈希", encoded)
			}
			if _, err := VerifyKey("s3cret", encoded, 0); err == nil {
				t.Fatal("坏哈希应该报错，不能当成口令不对")
			}
		})
	}
}

func TestVerifyKeyHonoursFloor(t *testing.T) {
	hashed, err := HashKey("s3cret")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	const floor = 60 * time.Millisecond
	for _, input := range []string{"s3cret", "wrong"} {
		started := time.Now()
		if _, err := VerifyKey(input, hashed, floor); err != nil {
			t.Fatalf("校验失败：%v", err)
		}
		if elapsed := time.Since(started); elapsed < floor {
			t.Fatalf("认证步骤只花了 %v，短于下限 %v", elapsed, floor)
		}
	}
}

func TestMessageViews(t *testing.T) {
	message := &Message{
		ID:        "message-2026-10-12-23-30-54-0852-abc123",
		Kind:      KindKey,
		Text:      "正文",
		KeyHash:   hashedTestKey(t),
		CreatedAt: time.Date(2026, 10, 12, 23, 30, 54, 852*int(time.Millisecond), time.Local),
		Attachments: []Attachment{
			{SHA256: strings.Repeat("a", 64), Name: "a.txt", Size: 3},
		},
	}
	view := message.View()
	if view.Locked || view.Text != "正文" || len(view.Attachments) != 1 {
		t.Fatalf("解锁视图不对：%+v", view)
	}
	if !strings.Contains(view.Attachments[0].URL, "?id="+message.ID) {
		t.Fatalf("附件地址没带消息 id：%q", view.Attachments[0].URL)
	}
	locked := message.LockedView()
	if !locked.Locked || locked.ID != message.ID || locked.CreatedAt != view.CreatedAt {
		t.Fatalf("锁定视图不对：%+v", locked)
	}
}

// hashedTestKey 给视图用例一个真实形状的哈希串。
func hashedTestKey(t *testing.T) string {
	t.Helper()
	hashed, err := HashKey("s3cret")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	return hashed
}
