#!/usr/bin/env bash
# 复现（API 可达版本）：MessageStore.Update 是「读整份 messageFile → 改字段 → 整份写回」，
# 与 Delete（写 isDeleted=true 后 Sweep 回收无人引用的附件）之间没有任何互斥。
# Update 读完旧快照后，Delete 先落盘并回收附件；Update 再用旧快照整份写回：
# 消息复活（isDeleted 又变回 false），附件却已经被回收，永久 404。
#
# 与 probe-message-race.sh 的区别：正文压到 1 MiB 以内（api.maxJSONBody = 1<<20 的真实上限），
# 并且跑多轮统计命中率，证明这条竞态在真实 HTTP 约束下可达，而不是靠 32 MiB 正文撑窗口。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-message-race-api.sh [轮数] [Delete 相对 Update 的延迟毫秒]
#
# 延迟只是把两个请求的重叠位置挪到 Update 的「读完、还没写回」的窗口里（这段窗口在 1 MiB 正文下约 5 ms）；
# 真实并发里两个 HTTP 请求的到达时刻本来就可以落在窗口内。轮数越多命中越多。
set -euo pipefail

repo="$(cd "$(dirname "$0")/../../.." && pwd)"
rounds="${1:-30}"
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
	"sync"
	"time"

	"github.com/hm-service/scope-status/store"
)

func main() {
	rounds := 30
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &rounds)
	}
	delay := 1000 * time.Microsecond
	if len(os.Args) > 2 {
		var millis int
		fmt.Sscanf(os.Args[2], "%d", &millis)
		delay = time.Duration(millis) * time.Millisecond
	}
	dir, err := os.MkdirTemp("", "message-race-api")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	st := store.NewMessageStore(filepath.Join(dir, "messages"), int64(1<<20))
	if err := st.Ensure(); err != nil {
		panic(err)
	}

	// api.readJSONBody 的上限是 1 MiB（含 JSON 外壳），正文留一点余量。
	text := strings.Repeat("x", (1<<20)-1024)

	resurrected := 0
	lostFile := 0
	attempts := 0
	var firstBad string
	for index := 0; index < rounds; index++ {
		body := fmt.Sprintf("attachment body %d", index)
		attachment, err := st.SaveFile("a.txt", strings.NewReader(body), -1)
		if err != nil {
			panic(err)
		}
		message, err := st.Create("", []store.AttachmentRef{{SHA256: attachment.SHA256, Name: attachment.Name}}, time.Now())
		if err != nil {
			panic(err)
		}
		attempts++

		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var updateErr, deleteErr error
		go func() {
			defer wait.Done()
			<-start
			_, updateErr = st.Update(message.ID, &text, nil, false, time.Now().UnixMilli())
		}()
		go func() {
			defer wait.Done()
			<-start
			time.Sleep(delay)
			_, deleteErr = st.Delete(message.ID)
		}()
		close(start)
		wait.Wait()

		_, getErr := st.Get(message.ID)
		alive := getErr == nil
		_, statErr := os.Stat(filepath.Join(st.FilesDir(), attachment.SHA256))
		fileGone := statErr != nil
		if alive {
			resurrected++
			if fileGone {
				lostFile++
			}
			if firstBad == "" {
				firstBad = fmt.Sprintf("round %d: updateErr=%v deleteErr=%v resurrected=%v attachmentGone=%v",
					index, updateErr, deleteErr, alive, fileGone)
			}
		}
	}
	fmt.Printf("attempts=%d resurrected=%d resurrectedWithLostAttachment=%d\n", attempts, resurrected, lostFile)
	if firstBad != "" {
		fmt.Printf("first bad case: %s\n", firstBad)
	}
	if resurrected == 0 {
		fmt.Println("没有复现到：本轮窗口没撞上（竞态依然存在，见代码：Update/Delete 对同一文件没有任何互斥）")
		os.Exit(3)
	}
}
GO

go run "$work/main.go" "$rounds" "${2:-1}"