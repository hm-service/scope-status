#!/usr/bin/env bash
# 复现：附件池里的孤儿文件会在「任意一次删除」触发的 Sweep 里被回收，
# 包括刚刚上传、用户还没点发送的那一个。上传与建消息之间没有占位/引用计数：
#
#   1) 用户 A 上传附件 foo（池子里多了一个 sha）；
#   2) 在 A 点发送之前，任何一个人删掉任何一条消息 → Delete 末尾的 Sweep 扫描全池，
#      foo 没有被任何未删除消息引用，于是被删掉；
#   3) A 点发送 → 服务端回 400「附件不在池子里」，上传白做；A 只能重传。
#
# 这是确定性的，不需要抢时序。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-sweep-orphan-upload.sh
set -euo pipefail

repo="$(cd "$(dirname "$0")/../../.." && pwd)"
work="$(mktemp -d)"
server=""
cleanup() {
    if [ -n "$server" ] && kill -0 "$server" 2>/dev/null; then
        kill "$server" 2>/dev/null || true
        wait "$server" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT

cd "$repo"
go build -trimpath -o "$work/scope-status" .

data="$work/data"
mkdir -m 700 "$data"
cat >"$data/source.yaml" <<'YAML'
probe:
  method: tcp
  timeout: 0.2
peers:
  - name: p
    kind: friend
    addresses: ["127.0.0.1:1"]
YAML

port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
base="http://127.0.0.1:$port"
auth="review_token_0123456789"
key="review_session_signing_key_0123456789"

AUTH_TOKEN="$auth" SESSION_SIGNING_KEY="$key" DATA_DIR="$data" HOST=127.0.0.1 PORT="$port" \
    STREAM_INTERVAL=60 MAX_API_CONCURRENCY=8 MAX_EVENT_CONCURRENCY=4 MAX_AUTH_CONCURRENCY=4 \
    "$work/scope-status" >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 50); do
    if curl -fsS -o /dev/null "$base/livez" 2>/dev/null; then break; fi
    sleep 0.1
done

session="$(curl -fsS -X POST -H 'Content-Type: application/json' \
    -d "{\"token\":\"$auth\"}" "$base/api/auth" | jq -r .token)"
[ -n "$session" ] && [ "$session" != "null" ]
auth_header=(-H "Authorization: Bearer $session")

echo "== 1) 上传附件（还没有任何消息引用它） =="
upload="$(curl -fsS -X PUT "${auth_header[@]}" -H 'Content-Type: application/octet-stream' \
    --data-binary 'orphan upload body' "$base/api/messages/files?name=orphan.txt")"
sha="$(printf '%s' "$upload" | jq -r .sha256)"
echo "uploaded sha=$sha"
echo "pool file exists: $(test -f "$data/messages/files/$sha" && echo yes || echo no)"

echo
echo "== 2) 另一个人删掉一条无关的旧消息（触发 Sweep） =="
victim="$(curl -fsS -X POST "${auth_header[@]}" -H 'Content-Type: application/json' \
    -d '{"text":"old message to delete"}' "$base/api/messages" | jq -r .id)"
echo "victim message=$victim"
removed="$(curl -fsS -X POST "${auth_header[@]}" "$base/api/messages/$victim/delete")"
echo "delete response removedFiles=$(printf '%s' "$removed" | jq -c '.removedFiles')"
echo "pool file after sweep: $(test -f "$data/messages/files/$sha" && echo yes || echo no)"

echo
echo "== 3) 上传者现在点发送 =="
create="$(curl -sS -w '\n%{http_code}' -X POST "${auth_header[@]}" -H 'Content-Type: application/json' \
    -d "{\"text\":\"with attachment\",\"attachments\":[{\"sha256\":\"$sha\",\"name\":\"orphan.txt\"}]}" \
    "$base/api/messages")"
code="$(printf '%s' "$create" | tail -1)"
echo "POST /api/messages -> HTTP $code"
echo "detail: $(printf '%s' "$create" | head -n -1 | jq -r '.detail // empty' 2>/dev/null || true)"
echo
if [ "$code" != "201" ]; then
    echo "结论：刚上传的附件被无关的删除操作清扫掉了，这次上传不可恢复，只能重传。"
else
    echo "结论：这次没被清掉（不应发生）"
fi