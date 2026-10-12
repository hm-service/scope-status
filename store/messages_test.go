package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

func newStore(t *testing.T, maxUpload int64) *MessageStore {
	t.Helper()
	store := NewMessageStore(filepath.Join(t.TempDir(), "messages"), maxUpload)
	if err := store.Ensure(); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	return store
}

func errorKey(t *testing.T, err error) i18n.Key {
	t.Helper()
	var business *core.Error
	if !errors.As(err, &business) {
		t.Fatalf("想要带文案键的业务错误，拿到 %v", err)
	}
	return business.Key
}

func create(t *testing.T, store *MessageStore, kind core.Kind, text, key string, refs []AttachmentRef) *core.Message {
	t.Helper()
	message, err := store.Create(kind, text, key, refs, time.Now())
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	return message
}

func TestCreateListAndOrder(t *testing.T) {
	store := newStore(t, 1024)
	first := create(t, store, core.KindPublic, "第一条", "", nil)
	time.Sleep(2 * time.Millisecond)
	second := create(t, store, core.KindPublic, "第二条", "", nil)

	messages, err := store.List()
	if err != nil {
		t.Fatalf("列表失败：%v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("消息数 = %d", len(messages))
	}
	if messages[0].ID != second.ID || messages[1].ID != first.ID {
		t.Fatalf("应该新的在前：%s / %s", messages[0].ID, messages[1].ID)
	}
	if !strings.HasPrefix(messages[0].ID, core.MessagePrefix) {
		t.Fatalf("id 形状不对：%q", messages[0].ID)
	}
}

func TestKeyMessageGate(t *testing.T) {
	store := newStore(t, 1024)
	message := create(t, store, core.KindKey, "秘密", "s3cret", nil)

	if _, err := store.Get(message.ID, "s3cret"); err != nil {
		t.Fatalf("口令对了还报错：%v", err)
	}
	if key := errorKey(t, mustErr(store.Get(message.ID, "nope"))); key != i18n.KeyMessageKeyWrong {
		t.Fatalf("口令错的错误键 = %q", key)
	}
	if key := errorKey(t, mustErr(store.Get(message.ID, ""))); key != i18n.KeyMessageKeyRequired {
		t.Fatalf("缺口令的错误键 = %q", key)
	}
	if key := errorKey(t, mustErr(store.Update(message.ID, ptr("改"), nil, false, "nope", 0))); key != i18n.KeyMessageKeyWrong {
		t.Fatalf("改口令消息也要验口令：%q", key)
	}
}

func TestCreateValidation(t *testing.T) {
	store := newStore(t, 1024)
	if key := errorKey(t, mustErr(store.Create("weird", "x", "", nil, time.Now()))); key != i18n.KeyMessageKindInvalid {
		t.Fatalf("kind 错的错误键 = %q", key)
	}
	if key := errorKey(t, mustErr(store.Create(core.KindKey, "x", "", nil, time.Now()))); key != i18n.KeyMessageKeyRequired {
		t.Fatalf("口令消息缺口令的错误键 = %q", key)
	}
	if key := errorKey(t, mustErr(store.Create(core.KindPublic, "   ", "", nil, time.Now()))); key != i18n.KeyMessageEmpty {
		t.Fatalf("空消息的错误键 = %q", key)
	}
	refs := []AttachmentRef{{SHA256: strings.Repeat("a", 64), Name: "a.txt"}}
	if key := errorKey(t, mustErr(store.Create(core.KindPublic, "x", "", refs, time.Now()))); key != i18n.KeyMessageFileNotInPool {
		t.Fatalf("池子里没有的附件应该被拒：%q", key)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	store := newStore(t, 1024)
	message := create(t, store, core.KindPublic, "原文", "", nil)

	updated, err := store.Update(message.ID, ptr("改过了"), nil, false, "", 1234)
	if err != nil {
		t.Fatalf("改失败：%v", err)
	}
	if updated.Text != "改过了" || updated.UpdatedAt == nil || *updated.UpdatedAt != 1234 {
		t.Fatalf("改的结果不对：%+v", updated)
	}
	if key := errorKey(t, mustErr(store.Update(message.ID, ptr("  "), nil, true, "", 1234))); key != i18n.KeyMessageEmpty {
		t.Fatalf("改成空消息应该被拒：%q", key)
	}

	if _, err := store.Delete(message.ID, ""); err != nil {
		t.Fatalf("删失败：%v", err)
	}
	if key := errorKey(t, mustErr(store.Get(message.ID, ""))); key != i18n.KeyMessageDeleted {
		t.Fatalf("删掉后应该 404：%q", key)
	}
	messages, err := store.List()
	if err != nil {
		t.Fatalf("列表失败：%v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("删掉的消息不该在列表里：%+v", messages)
	}
}

// 删除只回收没人引用的附件：还有别的消息引用着就得留着。
func TestDeleteSweepsOnlyUnreferencedFiles(t *testing.T) {
	store := newStore(t, 1024)
	body := strings.NewReader("shared content")
	attachment, err := store.SaveFile("共享.txt", body, 14)
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	refs := []AttachmentRef{{SHA256: attachment.SHA256, Name: attachment.Name}}
	first := create(t, store, core.KindPublic, "", "", refs)
	second := create(t, store, core.KindPublic, "", "", refs)

	removed, err := store.Delete(first.ID, "")
	if err != nil {
		t.Fatalf("删失败：%v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("还有消息引用着，不该回收：%v", removed)
	}
	if _, err := os.Stat(store.FilesDir() + "/" + attachment.SHA256); err != nil {
		t.Fatalf("附件不该被删掉：%v", err)
	}

	removed, err = store.Delete(second.ID, "")
	if err != nil {
		t.Fatalf("删失败：%v", err)
	}
	if len(removed) != 1 || removed[0] != attachment.SHA256 {
		t.Fatalf("没人引用了就该回收：%v", removed)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("统计失败：%v", err)
	}
	if stats.Files != 0 || stats.FileBytes != 0 || stats.Live != 0 || stats.Deleted != 2 {
		t.Fatalf("统计不对：%+v", stats)
	}
}

func TestSaveFileDedupesAndLimits(t *testing.T) {
	store := newStore(t, 16)
	first, err := store.SaveFile("a.txt", strings.NewReader("hello attachment"), 16)
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	second, err := store.SaveFile("b.txt", strings.NewReader("hello attachment"), 16)
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	if first.SHA256 != second.SHA256 || first.Name == second.Name {
		t.Fatalf("内容寻址没生效：%+v / %+v", first, second)
	}
	entries, err := os.ReadDir(store.FilesDir())
	if err != nil {
		t.Fatalf("读目录失败：%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("同样内容只该留一份：%v", entries)
	}
	if first.Size != 16 {
		t.Fatalf("大小 = %d", first.Size)
	}

	if key := errorKey(t, mustErr(store.SaveFile("c.txt", strings.NewReader("x"), 17))); key != i18n.KeyMessageDeclaredTooBig {
		t.Fatalf("声明的大小超限应该被拒：%q", key)
	}
	if key := errorKey(t, mustErr(store.SaveFile("c.txt", strings.NewReader(strings.Repeat("x", 17)), -1))); key != i18n.KeyMessageFileTooBig {
		t.Fatalf("边写边超限应该被拒：%q", key)
	}
	entries, err = os.ReadDir(store.FilesDir())
	if err != nil {
		t.Fatalf("读目录失败：%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("超限的中途文件要清掉：%v", entries)
	}
}

func TestReadFile(t *testing.T) {
	store := newStore(t, 1024)
	attachment, err := store.SaveFile("报告.txt", strings.NewReader("正文内容"), -1)
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	message := create(t, store, core.KindPublic, "", "", []AttachmentRef{{SHA256: attachment.SHA256, Name: "报告.txt"}})

	path, name, err := store.ReadFile(message.ID, attachment.SHA256, "")
	if err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	if name != "报告.txt" || filepath.Base(path) != attachment.SHA256 {
		t.Fatalf("下载信息不对：%q / %q", path, name)
	}
	if key := errorKey(t, mustErr2(store.ReadFile(message.ID, "not-a-sha", ""))); key != i18n.KeyMessageSHAFormat {
		t.Fatalf("sha 不合法：%q", key)
	}
	if key := errorKey(t, mustErr2(store.ReadFile(message.ID, strings.Repeat("b", 64), ""))); key != i18n.KeyMessageNotReferenced {
		t.Fatalf("没引用过的附件：%q", key)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("删附件失败：%v", err)
	}
	if key := errorKey(t, mustErr2(store.ReadFile(message.ID, attachment.SHA256, ""))); key != i18n.KeyMessageFileGone {
		t.Fatalf("附件没了：%q", key)
	}
}

func TestBrokenMessageFiles(t *testing.T) {
	cases := []struct {
		name string
		body string
		key  i18n.Key
	}{
		{"不是 JSON", "{", i18n.KeyMessageBrokenFile},
		{"顶层不是对象", "[]", i18n.KeyMessageFileNotObject},
		{"kind 不认识", `{"kind":"weird","text":"x","isDeleted":false,"attachments":[]}`, i18n.KeyMessageFileKind},
		{"缺 text", `{"kind":"public","isDeleted":false,"attachments":[]}`, i18n.KeyMessageFileFields},
		{"口令消息没 key", `{"kind":"key","text":"x","isDeleted":false,"attachments":[]}`, i18n.KeyMessageFileKey},
		{"attachments 不是数组", `{"kind":"public","text":"x","isDeleted":false,"attachments":null}`, i18n.KeyMessageFileAtts},
		{"附件条目不对", `{"kind":"public","text":"x","isDeleted":false,"attachments":[{"sha256":"x"}]}`, i18n.KeyMessageFileAttEntry},
		{"updatedAt 不是整数", `{"kind":"public","text":"x","isDeleted":false,"updatedAt":"now","attachments":[]}`, i18n.KeyMessageFileUpdatedAt},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := newStore(t, 1024)
			id := "message-2026-10-12-23-30-54-0852-abc123"
			if err := os.WriteFile(filepath.Join(store.Root(), id+core.MessageSuffix), []byte(testCase.body), 0o600); err != nil {
				t.Fatalf("写文件失败：%v", err)
			}
			_, err := store.List()
			if err == nil {
				t.Fatal("坏文件必须报错，不能跳过")
			}
			if key := errorKey(t, err); key != testCase.key {
				t.Fatalf("错误键 = %q，想要 %q（%v）", key, testCase.key, err)
			}
		})
	}
}

func TestPermissions(t *testing.T) {
	store := newStore(t, 1024)
	message := create(t, store, core.KindPublic, "x", "", nil)
	for _, path := range []string{store.Root(), store.FilesDir()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat 失败：%v", err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s 权限 = %v，想要 0700", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(store.Root(), message.ID+core.MessageSuffix))
	if err != nil {
		t.Fatalf("stat 失败：%v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("消息文件权限 = %v，想要 0600", info.Mode().Perm())
	}
}

func TestLoadSource(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSource(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Fatal("配置文件不在应该报错")
	} else if key := errorKey(t, err); key != i18n.KeyConfigMissing {
		t.Fatalf("错误键 = %q", key)
	}
	path := filepath.Join(dir, "source.yaml")
	if err := os.WriteFile(path, []byte("friends:\n  - name: 小明\n    addresses: [\"10.0.0.1\"]\n"), 0o600); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
	source, err := LoadSource(path)
	if err != nil {
		t.Fatalf("加载失败：%v", err)
	}
	if len(source.Friends) != 1 {
		t.Fatalf("friends = %d", len(source.Friends))
	}
}

func ptr(value string) *string { return &value }

func mustErr[T any](_ T, err error) error { return err }

func mustErr2[T any, U any](_ T, _ U, err error) error { return err }
