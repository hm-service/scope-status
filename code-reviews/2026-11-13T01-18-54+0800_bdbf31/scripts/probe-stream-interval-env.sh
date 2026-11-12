#!/usr/bin/env bash
# 复现：STREAM_INTERVAL 的校验只有「ParseFloat 能过 + 不小于 0」，
# NaN / Inf / 大到溢出的有限值都能通过并启动，只是在 main.go 里
# time.Duration(seconds * float64(time.Second)) 落到负值，SSE 循环静默不启动。
# 文档承诺「任何一个环境变量写错都是启动即失败」，对照 STREAM_INTERVAL=abc 确实会失败。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-stream-interval-env.sh
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

auth="review_token_0123456789"
key="review_session_signing_key_0123456789"
port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
base="http://127.0.0.1:$port"

run_fail_fast() {
    local value="$1" log="$work/fail.log"
    set +e
    AUTH_TOKEN="$auth" SESSION_SIGNING_KEY="$key" DATA_DIR="$data" HOST=127.0.0.1 PORT="$port" \
        STREAM_INTERVAL="$value" timeout 5 "$work/scope-status" >"$log" 2>&1
    local code=$?
    set -e
    echo "STREAM_INTERVAL=$value -> exit $code"
    sed -n '1,2p' "$log"
}

# 对每个可疑值：启动 → 看日志有没有 "server started" → 取 /api/stats 的 streamInterval → 杀掉
run_started() {
    local value="$1" log="$work/started-$2.log"
    AUTH_TOKEN="$auth" SESSION_SIGNING_KEY="$key" DATA_DIR="$data" HOST=127.0.0.1 PORT="$port" \
        STREAM_INTERVAL="$value" "$work/scope-status" >"$log" 2>&1 &
    server=$!
    for _ in $(seq 50); do
        if curl -fsS -o /dev/null "$base/livez" 2>/dev/null; then break; fi
        sleep 0.1
    done
    local started="没有启动"
    grep -q '"server started"' "$log" && started="启动成功"
    local interval="?"
    if [ "$started" = "启动成功" ]; then
        local session
        session="$(curl -fsS -X POST -H 'Content-Type: application/json' \
            -d "{\"token\":\"$auth\"}" "$base/api/auth" | jq -r .token)"
        interval="$(curl -fsS -H "Authorization: Bearer $session" "$base/api/stats" | jq -c '.streamInterval')"
    fi
    echo "STREAM_INTERVAL=$value -> $started，/api/stats.streamInterval=$interval（0 表示 SSE 循环没有启动）"
    kill "$server" 2>/dev/null || true
    wait "$server" 2>/dev/null || true
    server=""
}

echo "== 对照：非数字（预期启动即失败） =="
run_fail_fast abc

echo
echo "== 以下都应当按「写错」拒绝启动，实际都起来了，而且静默关掉了 SSE 循环 =="
run_started NaN nan
run_started Inf inf
run_started 1e10 overflow   # 秒，约 317 年；乘 1e9 转 int64 溢出