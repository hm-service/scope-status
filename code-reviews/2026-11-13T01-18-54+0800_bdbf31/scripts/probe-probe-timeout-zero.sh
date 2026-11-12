#!/usr/bin/env bash
# 复现：source.yaml 里 probe.timeout 只校验「是不是数字」，不校验正负与上界。
# timeout: 0 会一路传到 net.DialTimeout / http.Client.Timeout，而 0 在 Go 里表示「不设超时」：
# 一端只接受连接、永不回包的地址就能把整轮探测（以及 Monitor.Stream）卡到 OS 级超时。
#
# 对照：timeout: 0.2 同一目标 0.2 秒就回 down。
#
# 用法：bash code-reviews/2026-11-13T01-18-54+0800_bdbf31/scripts/probe-probe-timeout-zero.sh
set -euo pipefail

repo="$(cd "$(dirname "$0")/../../.." && pwd)"
work="$(mktemp -d)"
pids=()
cleanup() {
    for pid in "${pids[@]:-}"; do
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
    rm -rf "$work"
}
trap cleanup EXIT

cd "$repo"
go build -trimpath -o "$work/scope-status" .

free_port() {
    python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

# 只 accept、不写任何字节的 TCP 服务：HTTP 探测会一直等响应头。
python3 - "$work/stall.port" <<'PY' &
import socket, sys, time
srv = socket.socket()
srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
srv.bind(("127.0.0.1", 0))
srv.listen(16)
open(sys.argv[1], "w").write(str(srv.getsockname()[1]))
conns = []
while True:
    conn, _ = srv.accept()
    conns.append(conn)  # 攥着，不回任何东西
PY
pids+=($!)
for _ in $(seq 50); do [ -s "$work/stall.port" ] && break; sleep 0.05; done
stall_port="$(cat "$work/stall.port")"
echo "stalled TCP server on 127.0.0.1:$stall_port"

run_case() {
    local tag="$1" timeout_value="$2" data="$work/data-$1" port="$3"
    mkdir -m 700 "$data"
    cat >"$data/source.yaml" <<YAML
probe:
  method: http
  timeout: $timeout_value
  port: $stall_port
peers:
  - name: stall
    kind: friend
    addresses: ["127.0.0.1"]
YAML
    local base="http://127.0.0.1:$port"
    local auth="review_token_0123456789"
    local key="review_session_signing_key_0123456789"
    AUTH_TOKEN="$auth" SESSION_SIGNING_KEY="$key" DATA_DIR="$data" HOST=127.0.0.1 PORT="$port" \
        STREAM_INTERVAL=60 "$work/scope-status" >"$work/server-$tag.log" 2>&1 &
    local server=$!
    pids+=("$server")
    for _ in $(seq 50); do
        curl -fsS -o /dev/null "$base/livez" 2>/dev/null && break
        sleep 0.1
    done
    local session
    session="$(curl -fsS -X POST -H 'Content-Type: application/json' -d "{\"token\":\"$auth\"}" "$base/api/auth" | jq -r .token)"
    echo "== $tag: probe.timeout: $timeout_value =="
    local started result code
    started="$(date +%s%3N)"
    set +e
    result="$(curl -sS -o "$work/body-$tag" -w '%{http_code}' --max-time 5 \
        -H "Authorization: Bearer $session" "$base/api/status?force=1" 2>"$work/err-$tag")"
    code=$?
    set -e
    local elapsed=$(( $(date +%s%3N) - started ))
    if [ "$code" = "28" ]; then
        echo "GET /api/status?force=1 在 ${elapsed}ms 内没有返回（curl 超时，客户端放弃）；服务端仍在等探测"
    else
        echo "GET /api/status?force=1 -> HTTP $result，用时 ${elapsed}ms"
        echo "body: $(head -c 300 "$work/body-$tag")"
    fi
    echo
}

run_case zero 0 "$(free_port)"
run_case control 0.2 "$(free_port)"