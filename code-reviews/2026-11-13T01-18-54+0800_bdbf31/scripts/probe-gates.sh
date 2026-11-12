#!/usr/bin/env bash
# 复现：/api/events/（多一个尾斜杠）仍然匹配 SSE 路由，但并发闸门按 path 精确匹配，
# 于是这条 SSE 落到业务闸门 MAX_API_CONCURRENCY 上；把业务闸门占住后 /api/stats 排队到客户端放弃。
#
# 对照：/api/events（无尾斜杠）走事件闸门，同时 /api/stats 正常 200。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-gates.sh
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
    STREAM_INTERVAL=60 MAX_API_CONCURRENCY=1 MAX_EVENT_CONCURRENCY=1 MAX_AUTH_CONCURRENCY=1 \
    "$work/scope-status" >"$work/server.log" 2>&1 &
server=$!

for _ in $(seq 50); do
    if curl -fsS -o /dev/null "$base/livez" 2>/dev/null; then break; fi
    sleep 0.1
done

session="$(curl -fsS -X POST -H 'Content-Type: application/json' \
    -d "{\"token\":\"$auth\"}" "$base/api/auth" | jq -r .token)"
[ -n "$session" ] && [ "$session" != "null" ]

hold_sse() {
    local path="$1" tag="$2"
    curl -sS -N -D "$work/$tag.headers" --max-time 20 \
        -H "Authorization: Bearer $session" "$base$path" >"$work/$tag.body" 2>"$work/$tag.err" &
    echo $!
}

probe_stats() {
    local timeout="$1"
    curl -sS -o /dev/null -w '%{http_code} %{time_total}' --max-time "$timeout" \
        -H "Authorization: Bearer $session" "$base/api/stats" 2>/dev/null || true
}

echo "== 1) SSE 走 /api/events/（尾斜杠） =="
holder="$(hold_sse /api/events/ trailing)"
sleep 0.8
head -1 "$work/trailing.headers" | tr -d '\r'
head -c 120 "$work/trailing.body" | tr '\n' ' '
echo
echo "GET /api/stats（MAX_API_CONCURRENCY=1，2 秒上限）：$(probe_stats 2)"
echo "（000 表示请求在业务闸门排队到客户端放弃，说明这条 SSE 占了业务名额）"
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
sleep 0.5

echo
echo "== 2) 对照：SSE 走 /api/events（无尾斜杠） =="
holder="$(hold_sse /api/events normal)"
sleep 0.8
head -1 "$work/normal.headers" | tr -d '\r'
echo "GET /api/stats（MAX_API_CONCURRENCY=1，2 秒上限）：$(probe_stats 2)"
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
