#!/usr/bin/env bash
# 复现：POST /api/reload 对「配置能解析、但一轮探测失败」的新配置回 500，
# 可新配置其实已经生效（/api/stats 已是新条目数），而且旧快照被清成空。
# DESIGN 说 reload 失败时「旧配置继续生效」。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-reload.sh
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
  - name: 旧条目
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
    STREAM_INTERVAL=300 MAX_API_CONCURRENCY=8 MAX_EVENT_CONCURRENCY=4 MAX_AUTH_CONCURRENCY=4 \
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

echo "== 旧配置 =="
echo "GET /api/stats peers: $(curl -fsS "${auth_header[@]}" "$base/api/stats" | jq -c '.peers')"
echo "GET /api/status: $(curl -sS -o /dev/null -w '%{http_code}' "${auth_header[@]}" "$base/api/status")"

cat >"$data/source.yaml" <<'YAML'
probe:
  method: tcp
  timeout: 0.2
peers:
  - name: 新条目一
    kind: friend
    addresses: ["127.0.0.1:1"]
  - name: 新条目二（解析不了）
    kind: friend
    addresses: ["nope.invalid:1"]
YAML

echo
echo "== 新配置能解析，但有一条域名解析不了 =="
reload="$(curl -sS -w '\n%{http_code}' -X POST "${auth_header[@]}" "$base/api/reload")"
code="$(printf '%s' "$reload" | tail -1)"
detail="$(printf '%s' "$reload" | head -n -1 | jq -r '.detail // empty' 2>/dev/null || true)"
echo "POST /api/reload -> HTTP $code"
echo "detail: $detail"
echo "GET /api/stats peers（读的是当前生效的 source）: $(curl -fsS "${auth_header[@]}" "$base/api/stats" | jq -c '.peers')"
echo "GET /api/status（快照被清空时 204）: $(curl -sS -o /dev/null -w '%{http_code}' "${auth_header[@]}" "$base/api/status")"
echo "GET /api/status?force=1（新配置探测失败，整轮无结果）: $(curl -sS -o "$work/force.body" -w '%{http_code}' "${auth_header[@]}" "$base/api/status?force=1") $(head -c 160 "$work/force.body")"
echo "进程还活着（循环没有退出）: $(kill -0 "$server" 2>/dev/null && echo yes || echo no)"
