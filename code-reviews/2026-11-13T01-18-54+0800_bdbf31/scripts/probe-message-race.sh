#!/usr/bin/env bash
# 复现：MessageStore.Update 是「读整个 messageFile → 改字段 → 整份写回」，没有任何锁，
# 而且写回时会带上读到的 IsDeleted=false。与 Delete 并发时：
#   1) Delete 先写 isDeleted=true，并 Sweep 掉「已无人引用」的附件；
#   2) Update 随后用自己的旧快照整份写回，消息复活，附件却已经被回收。
# 结果：一条已被成功删除的消息重新出现，且它的附件永久丢失（下载 404）。
#
# 为了把窗口拉宽到可稳定复现，Update 的正文用了很大的字符串（写回时 JSON 序列化慢），
# 但竞态本身与正文大小无关：Update 在读与写之间没有任何再校验或互斥。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-message-race.sh
set -euo pipefail

repo="$(cd "$(dirname "$0")/../../.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$repo"

cat >"$work/main.go" <<'GO'
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hm-service/scope-status/store"
)

func main() {
	dir, err := os.MkdirTemp("", "message-race")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	st := store.NewMessageStore(filepath.Join(dir, "messages"), 1<<20)
	if err := st.Ensure(); err != nil {
		panic(err)
	}
	attachment, err := st.SaveFile("a.txt", strings.NewReader("attachment body"), -1)
	if err != nil {
		panic(err)
	}
	message, err := st.Create("", []store.AttachmentRef{{SHA256: attachment.SHA256, Name: attachment.Name}}, time.Now())
	if err != nil {
		panic(err)
	}
	fmt.Printf("create: id=%s attachments=%d\n", message.ID, len(message.Attachments))

	huge := strings.Repeat("x", 32<<20) // 只为拉宽 Update 的写回窗口
	updateDone := make(chan error, 1)
	go func() {
		_, err := st.Update(message.ID, &huge, nil, false, time.Now().UnixMilli())
		updateDone <- err
	}()
	// 让 Update 先读走旧的 messageFile（此时 IsDeleted=false），再停在 32MB 的 JSON 序列化里
	time.Sleep(5 * time.Millisecond)

	removed, err := st.Delete(message.ID)
	fmt.Printf("delete: err=%v removedFiles=%v\n", err, removed)

	updateErr := <-updateDone
	fmt.Printf("concurrent update: err=%v\n", updateErr)

	list, err := st.List()
	fmt.Printf("list after both: err=%v liveMessages=%d\n", err, len(list))
	if err != nil {
		return
	}
	for _, item := range list {
		fmt.Printf("  live id=%s attachments=%d textLen=%d\n", item.ID, len(item.Attachments), len(item.Text))
	}
	_, statErr := os.Stat(filepath.Join(st.FilesDir(), attachment.SHA256))
	fmt.Printf("attachment still in pool: %v (stat err=%v)\n", statErr == nil, statErr)
}
GO

go run "$work/main.go"
