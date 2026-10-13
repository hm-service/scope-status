package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

// 回归：消息 id 来自文件名，不出现在 json 里 —— 曾经把 id 当成 json 字段读取过。
func TestRegressMessageIDSnapshotFromFileName(t *testing.T) {
	store := newStore(t, 1024)
	message, err := store.Create(core.KindPublic, "正文", "", nil, time.Now())
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	raw, err := os.ReadFile(filepath.Join(store.Root(), message.ID+core.MessageSuffix))
	if err != nil {
		t.Fatalf("读消息失败：%v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("解析消息失败：%v", err)
	}
	if _, exists := fields["id"]; exists {
		t.Fatal("json 里不该有 id：id 就是文件名")
	}
	if _, exists := fields["updatedAt"]; exists {
		t.Fatal("没编辑过的消息不该落 updatedAt")
	}
	if _, exists := fields["key"]; exists {
		t.Fatal("普通消息不该有 key 字段")
	}
	info, err := os.Stat(filepath.Join(store.Root(), message.ID+core.MessageSuffix))
	if err != nil {
		t.Fatalf("stat 失败：%v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("消息文件权限 = %v", info.Mode().Perm())
	}
}

// 回归：上传必须带附件名，曾经会把空名字变成 "None" 存进池子。
func TestRegressUploadWithoutName(t *testing.T) {
	store := newStore(t, 1024)
	_, err := store.SaveFile("", nil, -1)
	if err == nil {
		t.Fatal("没有名字应该被拒")
	}
	if got := err.Error(); got == "" {
		t.Fatal("错误要说清原因")
	}
}

// 回归：孤儿附件（传了又没发）只在删除触发的清扫里被回收。
func TestRegressOrphanFileCollectedBySweep(t *testing.T) {
	store := newStore(t, 1024)
	orphan, err := store.SaveFile("孤儿.txt", strings.NewReader("orphan"), -1)
	if err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(store.FilesDir(), orphan.SHA256)); err != nil {
		t.Fatalf("刚上传的文件应该在池子里：%v", err)
	}
	removed, err := store.Sweep()
	if err != nil {
		t.Fatalf("清扫失败：%v", err)
	}
	if len(removed) != 1 || removed[0] != orphan.SHA256 {
		t.Fatalf("孤儿附件应该被回收：%v", removed)
	}
}

// 回归：口令消息只存 Argon2id 哈希，盘上（包括附件池）不出现明文口令。
func TestRegressKeyNeverLandsInPlaintext(t *testing.T) {
	store := newStore(t, 1024)
	message := create(t, store, core.KindKey, "秘密", "s3cret-plaintext", nil)
	path := filepath.Join(store.Root(), message.ID+core.MessageSuffix)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读消息失败：%v", err)
	}
	if bytes.Contains(raw, []byte("s3cret-plaintext")) {
		t.Fatal("明文口令落盘了")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if _, exists := fields["key"]; exists {
		t.Fatal("不该再有 key 字段")
	}
	var hash string
	if err := json.Unmarshal(fields["keyHash"], &hash); err != nil {
		t.Fatalf("keyHash 读不出来：%v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") || !core.ValidKeyHash(hash) {
		t.Fatalf("keyHash 不是 Argon2id 的 PHC 串：%q", hash)
	}
	if _, err := store.Get(message.ID, "s3cret-plaintext"); err != nil {
		t.Fatalf("明文口令仍然应该能解锁：%v", err)
	}
	if key := errorKey(t, mustErr(store.Get(message.ID, "s3cret-plaintex"))); key != i18n.KeyMessageKeyWrong {
		t.Fatalf("少了最后一个字符也该拒绝：%q", key)
	}
}
