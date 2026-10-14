package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/core"
)

// 回归：消息 id 来自文件名，不出现在 json 里 —— 曾经把 id 当成 json 字段读取过。
func TestRegressMessageIDSnapshotFromFileName(t *testing.T) {
	store := newStore(t, 1024)
	message, err := store.Create("正文", nil, time.Now())
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
	// 消息不分类型：盘上不该再有 kind / key 这类字段
	for _, field := range []string{"kind", "key", "keyHash"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("消息文件里不该有 %s 字段", field)
		}
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
