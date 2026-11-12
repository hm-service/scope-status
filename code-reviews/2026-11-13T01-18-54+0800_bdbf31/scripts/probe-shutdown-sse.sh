#!/usr/bin/env bash
# 复现：只要有任意一个 SSE 客户端连着，SIGTERM 的「优雅退出」每次都跑满 5 秒宽限期并被强制关掉，
# 进程以 254 退出、日志里留一条 ERROR "server shutdown forced"。
#
# 原因：http.Server.Shutdown 不会取消在途请求的 context；handleEvents 要等客户端断开才返回，
# 于是 Shutdown 一直等到宽限期超时，最后 server.Close() 才把连接掐掉。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-shutdown-sse.sh
set -euo pipefail

repo="$(cd "$(dirname "$0")/../../.." && pwd)"
work="$(mktemp -d)"
server=""
sse=""
cleanup() {
    for pid in "$sse" "$server"; do
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
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

# 一条普通的、行为良好的 SSE 客户端（浏览器页面就是这么连的）
curl -sS -N -H "Authorization: Bearer $session" "$base/api/events" >"$work/sse.body" 2>/dev/null &
sse=$!
sleep 0.5
if [ ! -s "$work/sse.body" ]; then
    echo "SSE 没连上，探针无效"
    exit 1
fi

echo "SSE 已建立，发送 SIGTERM ..."
started="$(date +%s%3N)"
kill -TERM "$server"
set +e
wait "$server"
code=$?
set -e
server=""
elapsed=$(( $(date +%s%3N) - started ))

echo "退出码: $code（0 = 优雅退出；254 = 被强制关掉）"
echo "从 SIGTERM 到进程退出: ${elapsed}ms"
echo "日志尾部:"
tail -3 "$work/server.log"
echo
if [ "$code" = "254" ]; then
    echo "结论：开着 SSE 时优雅退出必然退化成强制关闭（固定等满 5s 宽限期）。"
else
    echo "结论：这次是正常退出。"
fi