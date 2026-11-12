# Review — scope-status @ bdbf312

- **被审 commit**：`bdbf31246da9057b137dd90b3bb3d1d0f6563d4d`（`bdbf312`，`升级工具链：go 1.27.4 / node 26.13.0 / pnpm 12.18.0`）
- **分支**：`main`
- **审查时间**：2026-11-13 01:29–01:52 +0800（Asia/Shanghai）
- **审查范围**：全项目。后端 `main` / `api` / `app` / `core` / `store` / `httpx` / `i18n`、`frontend/src`（Vue 组件、SSE、store、router/entry、i18n），以及 `DESIGN.md`、`README.md`、`source.example.yaml`、`Dockerfile`、CI、`justfile`、`k8s.scope-status.yaml`。变更面按最近提交取：`bdbf312`（工具链）、`b9a872f`（SSE 闸门 / UDP 探测 / 附件下载）、`92e5eae`、`ce944eb`、`e8f7a1e`、`316845c`。
- **轮次**：该项目第一轮 review（`code-reviews/` 之前不存在，没有上一轮修复可对比；`b9a872f` 的三处线上修复在本轮按"实际修复是否正确"核实）。
- **开始时 git 状态**：`git status --porcelain` 只有 `?? code-reviews/`；工作树干净，HEAD = `bdbf312`。本目录下 `scripts/` 里的探针是 review 产物，不属于被审变更。
- **环境**：`go version go1.27.4 linux/amd64`；前端类型检查用仓库内 `frontend/node_modules/.bin/vue-tsc`。未接触生产环境，未打印任何密钥。

## 结论

| 严重度 | 数量 | 编号 |
| --- | --- | --- |
| P0 | 1 | F1 |
| P1 | 1 | F2 |
| P2 | 6 | F3–F8 |
| P3 | 6 | F9–F14 |

最该先修：**F1**。`MessageStore.Update` 与 `Delete` 之间没有任何互斥，一次并发的"编辑 + 删除"可以同时成功，结果删除被静默撤销、附件被永久回收；这是数据丢失，且同类无锁问题还覆盖 Sweep、并发 PATCH。

---

## Findings

### [P0][F1] 消息 Update / Delete 无锁：删除被复活，附件被 Sweep 永久回收

- **位置**：`store/messages.go:304-340`（`Update` / `Delete`）、`store/messages.go:579-596`（同一个 `.<id>.json.tmp` 临时文件）、`store/messages.go:343-377`（`Sweep`）、`app/messages.go:60-74`
- **触发条件**：两个并发请求打到同一条消息，例如 A 打开编辑框点保存（`PATCH /api/messages/{id}`）的同时 B 删除这条消息（`POST /api/messages/{id}/delete`）。`Update` 先 `locate()`（读整份 json）再修改再整份写回；`Delete` 也是读整份 json、置 `isDeleted=true` 写回、然后 `Sweep` 回收无人引用的附件。两者都没有任何锁或版本校验。
- **实际结果**：两个请求都返回成功，但删除被撤销：`Update` 拿着"未删除"的旧快照整份写回，消息重新出现在列表里；而 `Delete` 的 `Sweep` 已经在 `Update` 写回之前把附件删掉（或随后看到已复活的消息而留下一个下载 404 的引用）。附件池按内容寻址、删掉后不可恢复。
  - `scripts/probe-message-race.sh`（store 层，32 MiB 正文撑窗口）：`delete: err=<nil> removedFiles=[baeb…]`、`concurrent update: err=<nil>`、`list after both: liveMessages=1`、`attachment still in pool: false`。
  - `scripts/probe-message-race-api.sh`（正文压到 API 上限 1 MiB 以内）：两个请求同时起跑（不加人为偏移）30 轮里本轮也命中了 1 次（上一次 0 次）；把 Delete 相对 Update 延后 1 ms（落在 Update 读完、还没写回的窗口里）后 `attempts=30 resurrected=27 resurrectedWithLostAttachment=1`。真实并发请求的到达时刻本来就可以落在这个窗口内。
  - 同源问题：两个并发 `PATCH` 用同一个临时文件名 `.<id>.json.tmp`，可能互相截断/覆盖（一个 rename 成功、另一个 rename 拿不到文件回 500），也不会做"后写覆盖前写"之外的任何合并。
- **修复建议**：给 `MessageStore` 加一把互斥锁（或按消息 id 分片的锁），把"读 → 改 → 写 → Sweep"整段收进临界区；写回前重新校验 `IsDeleted`（乐观并发：读到的 `updatedAt/IsDeleted` 与当前不一致就拒绝）；临时文件名带上随机后缀，保证并发写不会落到同一个路径。多副本部署如果也要支持，则需要文件锁或版本号，不能只靠进程内锁。补一个并发用例（并发 PATCH+DELETE 后断言消息不可见、附件引用一致）。

### [P1][F2] reload 失败（探测失败）时新配置已生效、旧快照被清空，却回 500

- **位置**：`app/monitor.go:215-226`（`Reload` 先换 `source`、清 `snapshot`，再返回）、`api/status.go:59-69`（换完再 `Refresh()`，失败就把 500 回给前端）、`app/monitor.go:178-212`（`Refresh` 失败不会回滚 `source`）
- **触发条件**：新配置能解析，但其中某个主机解析不了（`peer` / `service` / `link` 的域名拼错、或 DNS 暂时不可用）。`POST /api/reload` 先 `Reload()` 成功（新 source 已生效、旧快照置 nil），随后 `Refresh()` 因 DNS 错误返回 error。
- **实际结果**：HTTP 500 + 新配置已生效 + 旧快照已清空，三者同时成立。`DESIGN.md:114` 明确写「失败时回 `500`，`detail` 就是配置校验的报错原文，**旧配置继续生效**」。页面点「重载配置」只看到红色报错，随后看板变成空（`/api/status` 回 204）、且如果探测一直失败就永远不恢复；`/api/stats` 已经显示新条目数，用户无法判断现在到底生效的是哪一份。
  - `scripts/probe-reload.sh` 实测：`POST /api/reload -> HTTP 500`、`detail: nope.invalid:1 解析失败`、`GET /api/stats peers: 2`（新配置）、`GET /api/status: 204`（旧快照已没）、`GET /api/status?force=1 -> 500 {"detail":"nope.invalid:1 解析失败"}`。
  - 现有用例 `api/api_test.go:214-227` / `app/monitor_test.go:252-291` 只覆盖了"YAML 解析失败 → 旧配置保留"，没有覆盖"解析成功、探测失败"。
- **修复建议**：把 reload 改成"先解析、先探一轮成功的快照，再一次性换 source + snapshot"；或者失败时回滚 `m.source`/`m.snapshot` 并保持旧快照可读，只把探测错误作为 200 响应里的警告（或者单独的状态字段）返回。至少不要出现"回了 500 但状态已变"的语义。补一条"新配置解析通过但探测失败"的用例，断言旧配置/旧快照仍然生效。

### [P2][F3] `/api/events/`（多一个尾斜杠）绕过事件闸门，吃掉业务闸门名额，可复现线上全站 503

- **位置**：`api/server.go:117-132`（`gate()` 按 `r.URL.Path` 字面量匹配 `"/api/events"`）、`api/router.go:127-131`（`matchSegments` 会 `strings.Trim(path, "/")`，所以 `/api/events/` 仍然路由到 SSE handler）
- **触发条件**：任何已登录的客户端（或加了尾斜杠的反代规则）请求 `/api/events/`。路由把它当 SSE，闸门却把它当业务 `/api/*`。
- **实际结果**：这条 SSE 占住一个 `MAX_API_CONCURRENCY` 名额直到客户端断开；占满后 `/api/stats`、`/api/status`、`/api/messages` 全部排队到客户端放弃，正是 `b9a872f` 要修的那个故障（只是从 128 条 `/api/events` 变成 128 条 `/api/events/`）。事件闸门本身是好的。
  - `scripts/probe-gates.sh` 实测（`MAX_API_CONCURRENCY=1`、`MAX_EVENT_CONCURRENCY=1`）：`/api/events/` 建立后 `GET /api/stats` 2 秒无响应（curl 状态 `000`）；对照 `/api/events` 后 `GET /api/stats` 立刻 `200`。
  - `api/concurrency_test.go:14` 的用例只测了不带尾斜杠的路径，所以没有发现。
- **修复建议**：不要在 `limit()` 里用"路径字符串"二次判定路由。最直接的做法是让路由表声明每条路由用哪道闸门，或者先把路径规范化（`strings.TrimSuffix(path, "/")`，并注意 `//`）再查表。补一条 `/api/events/` 的回归用例，断言它走事件闸门（占满事件闸门后业务端点仍然 200，或第二条 `/api/events/` 排队超时）。

### [P2][F4] `STREAM_INTERVAL=NaN` / `Inf` / 溢出值通过校验并启动，静默关掉 SSE 循环

- **位置**：`main.go:225-231`（只判 `< 0`）、`main.go:281`（`time.Duration(streamSeconds * float64(time.Second))`）、`main.go:309-319`（`envFloat` 直接 `strconv.ParseFloat`）
- **触发条件**：`STREAM_INTERVAL=NaN`、`Inf`，或大到超出 `int64` 纳秒的有限值（如 `1e10` 秒）。
- **实际结果**：三种值都启动成功；`ParseFloat` 通过、`< 0` 为假，`float64 → int64` 转换溢出得到负的 `time.Duration`，`main.go:120` 的 `cfg.StreamInterval > 0` 不成立，SSE 循环静默不启动；`/api/stats.streamInterval` 为 0，前端只收到 `hello`，看板不再自动更新，日志里没有任何提示。`README.md:183` 承诺「任何一个环境变量写错都是启动即失败，不猜」。
  - `scripts/probe-stream-interval-env.sh` 实测：`abc` → exit 1；`NaN` / `Inf` / `1e10` → 启动成功、`/api/stats.streamInterval=0`。
- **修复建议**：`envFloat` 拒绝 `NaN`/`±Inf`；`STREAM_INTERVAL` 增加有限上界（例如 `0 ≤ v ≤ 86400`），换算成 `time.Duration` 前显式判界（`if v > float64(math.MaxInt64)/float64(time.Second)` 直接报错），不要依赖浮点转整数的实现定义行为。补上 `NaN`/`Inf`/超界三条用例。

### [P2][F5] `probe.timeout: 0` 被接受，而 0 在 Go 里等于「不设超时」，一个挂住的地址能卡死整轮探测

- **位置**：`core/config.go:372-378`（只校验"是数字"）、`core/probe.go:110`（`net.DialTimeout(..., config.Timeout)`）、`core/probe.go:131-132`（`http.Client{Timeout: config.Timeout}`）
- **触发条件**：`probe.timeout: 0` 配合一个只接受连接、不回响应的地址（负值走另一条路：`http.Client` 立即判超时，每轮都把所有条目打成 down）。
- **实际结果**：`net.DialTimeout` / `http.Client.Timeout` 传 0 表示"没有超时"。该探测会一直挂到 OS 级超时，`Monitor.collect()` 的 `wait.Wait()` 因此不返回，SSE 循环整轮被拖住，`/api/status?force=1` 也一起阻塞（请求一直挂着，直到客户端放弃）；页面表现为"永远没有快照"。
  - `scripts/probe-probe-timeout-zero.sh` 实测（本地 stalled TCP server，`probe.method: http`）：`timeout: 0` 时 `GET /api/status?force=1` 5 秒内没有返回（curl 超时放弃，服务端仍在等）；`timeout: 0.2` 对照 209 ms 返回 200 + `down`。
  - 同一处配置还缺上界：`probe.port` 没有 1–65535 校验（`core/config.go:379-385`），`links.expect_status` 没有 100–599 校验（`core/config.go:772-778`）；它们只会导致条目标 down / 永远 mismatch，不会挂住，但也属于"配置语义静默接受非法值"。
- **修复建议**：`timeout` 要求 `0 < v ≤ N`（例如 60 秒）且换算成 `time.Duration` 后为正；`probe.port` 复用 service port 的 1–65535 校验；`expect_status` 限定在合法状态码区间。补配置解析用例。

### [P2][F6] 任何一条地址 DNS 解析失败会让整轮探测丢弃所有结果（看板永久空白），而 ICMP 方法下同样的失败只是一个 down

- **位置**：`core/probe.go:115-117`（tcp）、`135-137`（http）、`176-177`（service tcp）、`197-199`（udp）、`306-308`（link）在 `isDNSError` 时返回 `core.Error`（而不是一个 down 结果）；`app/monitor.go:382-384` 只要有任一 job 返回 error 就整轮失败、`Refresh` 不更新快照。
- **触发条件**：配置里任意一个 `peers[].addresses`、`services[].host`、`links.url/probe_url` 的域名解析不了（拼错、内部 DNS 暂时不可用），且 `probe.method` 不是 `icmp`。
- **实际结果**：整轮探测没有任何结果被保留，`/api/status?force=1` 回 500、`/api/status` 在有循环时回 204、SSE 永远不推 `snapshot`；看板整块空白，只有服务端日志里有一条 `stream loop stopped`。对比：ICMP 方法下 `ping` 的 unknown host 会被折成该地址的 `down` + `DNS lookup failed`（`core/probe.go:93-105`），其它条目照常显示。`source.example.yaml:62` 的注释还停留在旧行为（"会把 SSE 循环直接停掉"），与 `b9a872f` 之后"循环不退出、只是这轮没快照"不一致。
- **修复建议**：探测层的 DNS 失败应当和其它网络失败一样产出"该条目 down + detail"，而不是让整轮失败（或者在 `collect` 里把单条 error 折成一个带 detail 的 down 结果，只有"配置/内部错误"才让整轮失败）。若确实想保留"硬错误"，至少保留上一份快照并让 UI 能看到错误，而不是 204 空白。顺手更新 `source.example.yaml:62` 的注释。

### [P2][F7] 刚上传、还没被消息引用的附件会被任意一次无关删除的 Sweep 回收

- **位置**：`store/messages.go:380-422`（上传只把文件放进池子，不登记"待引用"）、`store/messages.go:343-377`（`Sweep` 回收所有"当前没有未删除消息引用"的池内文件）、`store/messages.go:330-340`（每次 Delete 都触发 `Sweep`）
- **触发条件**：用户 A 上传附件后还没点发送；此时任何一个人删除任意一条消息（A 自己删另一条也算），触发 `Sweep`。
- **实际结果**：A 刚上传的文件被删掉，A 点发送时服务端回 `400 附件 … 不在池子里，先上传`；更糟的是删除者收到的 `removedFiles` 里出现一个自己完全不认识的 sha，而 A 的这次上传不可恢复（只能重传）。若 `Sweep` 恰好发生在 `Create/Update` 的 `resolveAttachments()` 之后、写回之前，还会写出一个引用已删除文件的消息（附件永久 404）。
  - `scripts/probe-sweep-orphan-upload.sh` 实测（确定性，不抢时序）：上传 `orphan.txt` → 删除一条无关消息 → 删除响应 `removedFiles=["41f0…"]`、池内文件消失 → `POST /api/messages` 回 `400 附件 … 不在池子里，先上传`。
  - `README.md:249` / `DESIGN.md:282` 把"孤儿附件等下一次删除再收"写成了设计，但没有说明它会误伤"正在编辑、还没发送"的上传。
- **修复建议**：Sweep 只回收"确实属于某条已删除消息、且不再被引用"的文件，而不是全池扫"没被引用"的文件（更直接，也符合"删除时回收"的语义）；或者上传时写一个带时间的 pending 标记，Sweep 只回收超过 N 分钟仍无引用的孤儿。补一条"上传 → 删其它消息 → 发送"的用例。

### [P2][F8] 前端 SSE 静默检测用固定 30 秒阈值、且不把 ping 帧算作存活：`STREAM_INTERVAL>30` 或探测持续失败会误报并反复重连

- **位置**：`frontend/src/store.ts:12`（`STREAM_SILENCE_MS = 30000`）、`frontend/src/store.ts:299-305`（`checkStreamHealth`）、`frontend/src/store.ts:278-288`（只有 `snapshot`/`hello` 更新 `lastFrameAt`）、`frontend/src/sse.ts:93-105`（注释帧直接丢弃，不回调）
- **触发条件**：`STREAM_INTERVAL > 30`（例如文档里允许的 `60`），或服务端探测持续失败（F6）导致长时间没有 `snapshot`；服务端每 15 秒发的 `: ping` 保活帧不算作"收到推送"。
- **实际结果**：每 30 秒的 `checkStreamHealth` 判定"SSE 静默"→ `setError(t('stream.silent'))` + `closeStream(); connect()`，即使连接完全健康也反复断线重连；而且 `state.error` 只在 `setError` 里被赋值、代码里没有任何 `setError(null)`，这条错误提示会一直挂在页脚，直到整页刷新。`docs/reverse-proxy.md` 还专门说明"应用每 15s 发一个注释帧保活"，但前端不把它当存活信号。
  - 该条为代码静态推得（未跑浏览器 E2E）：`STREAM_INTERVAL=60` 下第 30 秒必然满足 `Date.now()-lastFrameAt > 30000`。
- **修复建议**：让 `dispatch` 把注释帧/任意帧都上调一次"最后活动时间"（或单独的 `onPing`）；静默阈值按 `max(3×streamInterval, 45s)` 计算，且服务端无快照（`hello` 之后一直没 snapshot）时不要把它当成断线；收到任意新帧时清除静默错误。

### [P3][F9] `STREAM_INTERVAL=0`（文档承诺的按需探测模式）下前端从不主动探一次，看板一直空白

- **位置**：`frontend/src/store.ts:491-498`（`initApp` 只 `connect()`，不调用 `fetchStatus`）、`frontend/src/api.ts:66-74`、`main.go:120-122`（`STREAM_INTERVAL=0` 时不起循环）
- **触发条件**：`STREAM_INTERVAL=0`。这也是 F4 里 NaN/Inf 落到的那条路径。
- **实际结果**：SSE 连上后服务端只发一帧 `messages` 和 `hello{streamInterval:0}`，`GET /api/status` 的"按需探测"逻辑（`app/monitor.go:163-175`）完全可用，但前端没有任何地方在启动时调用它；用户看到的是空看板（`empty.waiting` / 顶栏 "connecting"），必须手动点一次「刷新」才有数据。`README.md:175` 把 `0` 写成可用模式。
- **修复建议**：`initApp` 里在没有快照时调一次 `fetchStatus()`（或在 `onHello` 且 `streamInterval === 0` 时调）；也可以在收到 `hello` 后加一个短超时（如 2 秒）仍无快照就主动 `fetchStatus(false)`。

### [P3][F10] 只要有任意一个 SSE 客户端连着，SIGTERM 的优雅退出每次都跑满 5 秒并被强制关闭（exit 254 + ERROR 日志）

- **位置**：`main.go:157-166`（`server.Shutdown(shutdownCtx)` → 超时 → `server.Close()` → `exitShutdownForced`）、`api/events.go:57-74`（`handleEvents` 只等 `r.Context().Done()`）
- **触发条件**：进程收到 SIGINT/SIGTERM 时有任何浏览器/客户端开着 `/api/events`（正常使用下的常态）。
- **实际结果**：`http.Server.Shutdown` 不会取消在途请求的 context，SSE handler 不返回，Shutdown 等满 `shutdownGrace = 5s` 后返回 `context deadline exceeded`，进程打 ERROR `server shutdown forced` 并以 254 退出；滚动更新/`just stop` 每次都变成"强制关闭"，日志与退出码无法区分真正的异常。
  - `scripts/probe-shutdown-sse.sh` 实测：SSE 建立后 SIGTERM → 退出码 `254`，耗时 `5009ms`，日志有 `server shutdown forced ... context deadline exceeded`。
- **修复建议**：用一个服务级 context 包 SSE（`http.Server.BaseContext` 或 `RegisterOnShutdown` 里 cancel），让 Shutdown 时 SSE handler 立即返回；或者把 SSE 连接按"长连接"处理（Shutdown 前主动 `Close`），不要把"有看客在线"当成退出失败。

### [P3][F11] 变更已落盘后 `publish()`/`Sweep()` 出错会让接口回 500，客户端重试会制造重复数据

- **位置**：`app/messages.go:52-74`（先 `store.Create/Update/Delete`，成功后再 `publish()`）、`app/messages.go:80-87`（`publish` 里重新 `Payload()` → `store.List()` 读全部消息，任一文件出错就失败）、`store/messages.go:330-340`（Delete 先写 `isDeleted` 再 `Sweep`，Sweep 出错也整个返回 error）
- **触发条件**：消息目录里存在一个不合规/读不了的 `message-*.json`（`DESIGN` 明确这种文件会 500），或 Sweep 删文件时遇到 IO 错误。
- **实际结果**：`POST /api/messages` 已经写出文件却回 500，用户重试会再建一条（重复消息）；`DELETE` 已经打了删除标志却回 500，重试拿到 404；`PATCH` 同理。错误语义与"操作是否生效"脱节。
- **修复建议**：`publish()` 的失败只记日志、不要覆盖已成功的变更结果（SSE 推帧失败不该等于写失败）；`Delete` 把 Sweep 的错误合并进响应的 warning 字段（或只记日志），保持 `isDeleted` 已生效的语义。补一个"目录里有坏文件时创建消息仍回 201"的用例。

### [P3][F12] `connect_cmd` 的复制按钮在明文 HTTP 内网下必然失败

- **位置**：`frontend/src/components/CopyButton.vue:11-22`
- **触发条件**：站点以 `http://10.x.x.x:8080` 明文访问（`DESIGN.md:90` 明确支持这种部署，CSP 还专门为它去掉了 `upgrade-insecure-requests`）。
- **实际结果**：`navigator.clipboard` 只在安全上下文（HTTPS / localhost）存在，明文 HTTP 下 `navigator.clipboard.writeText` 抛错，按钮只会弹"复制失败"；`connect_cmd` 的可复制功能在最主要的内网部署里不可用（代码注释里也承认"http 下没有 clipboard 也要说一声"）。
- **修复建议**：失败时回退到"临时 textarea + `document.execCommand('copy')`"，或者把命令渲染成可选中的文本并在不可复制时提示手动选择。

### [P3][F13] 消息数量/正文没有上限，且每次变更都把整份消息列表读一遍并推给每个订阅者

- **位置**：`core/messages.go:13`（注释明确"消息条数、正文长度、附件个数都不设限"）、`app/messages.go:29-50`（`PayloadSince` 每次都 `store.List()` 读全部消息）、`app/messages.go:80-87`（每次增删改都广播整份列表）、`app/monitor.go:15,58-70`（每个订阅者最多积压 4 帧完整列表）、`store/messages.go:189-212`（`List` 逐文件读取+解析）
- **触发条件**：消息数量或单条正文（API 上限 1 MiB）增长到较大规模，或订阅者较多。
- **实际结果**：每次 `POST/PATCH/DELETE` 都要 O(n) 读盘 + JSON 解析 + 向每个 SSE 连接各序列化一次整份列表；`/api/stats`、`/readyz` 也会把全部消息读一遍。`GOMEMLIMIT=128MiB`、Pod limit 256MiB 的部署下，消息一多内存与 CPU 会明显上升（未压测），`/readyz` 可能因读盘变慢而 503。`DESIGN.md:313` 又声称"应用只做输入上限与校验（体积、字段长度、类型）"，与"不设限"不一致。
- **修复建议**：给消息条数/单条正文设明确上限（配置项或在 DESIGN 里写死），或把列表接口做分页/区间；SSE 推增量（新增/修改/删除的具体条目）而不是每帧整份列表；至少在 DESIGN 里把"不设限"的代价写清楚。

### [P3][F14] 死代码与重复实现：`Category.Label` 永不赋值、前端分组回退不可达、分组逻辑前后端各一套

- **位置**：`core/config.go:84-100`（`Category.Label` 分支）、`core/config.go:303`（`Categories` 只由 `usedCategories`/`builtinCategory` 产生，配置里没有 `label` 字段可读）、`frontend/src/store.ts:159-183`（按 `payload.categories` 的兜底分组）
- **触发条件**：静态可读。
- **实际结果**：`Category.Label` 的两个分支（`return c.Label`）永远不会被执行；`api/view.go` 的 `snapshotView` 根本没有 `categories` 字段，`resolveGroups` 的兜底分支在当前后端下不可达，等于同一套分组规则在 `core.groupBy` 和前端各维护了一份。改动分组规则时容易只改一边。
- **修复建议**：删掉 `Category.Label` 与前端不可达的分组兜底（或者反过来：后端不分组、前端只保留一份），让分组只有一个事实来源。

---

## 待确认（不算 finding，缺证据或影响需产品判断）

1. **`i18n.Negotiate` 不处理 `q=0`**（`i18n/i18n.go:441-477`）：`Accept-Language: en;q=0.9, zh;q=0` 仍会选中 `zh`（因为 `q=0 > bestQ=-1`），`*` 也固定映射到默认 `zh`。只影响服务端错误/探测文案的语言，未按 RFC 语义验证，暂不列为 finding。
2. **`MAX_UPLOAD_BYTES` 大于 1 GiB、`MAX_*_CONCURRENCY` 没有上界**：只确认了校验缺失；`chan struct{}` 的缓冲不按元素大小分配，没实测到具体资源后果。
3. **附件下载 `fetch` 全量进 Blob**（`frontend/src/api.ts:140-153`）：1 GiB 附件会让浏览器把整份内容放进内存/临时盘，没有流式保存；`URL.revokeObjectURL` 在 `setTimeout(0)` 里回收，Firefox 上是否偶发空文件未实测。
4. **`/readyz` 的权限断言会读全部消息与附件**（`app/app.go:56-71`、`store/messages.go:146-175`）：消息多了以后探针变慢/变贵的程度未压测。
5. **后台标签页恢复时 `checkStreamHealth` 的误判**：浏览器定时器节流会让 `lastFrameAt` 显得更旧，恢复瞬间是否必然误报一条"静默"未在真浏览器验证（与 F8 同源）。
6. **前端 `resolveGroups` 之外的旧字段**（`categories`、`reloadable`）是否还有外部使用方，未确认，不能断言可以直接删。

---

## 近期修复的核实结果

| 声明 | 结论 | 依据 |
| --- | --- | --- |
| 整站 CSP，`script-src 'self'` | **成立** | `httpx/security.go:15-17`；`frontend/index.html` / `auth.html` 只有同源 `/theme.js` 与 `/src/*.ts`，`main_test.go` 有用例钉住；CSP 也挂在 401/错误响应上（`api/security_test.go`） |
| 附件下载改 fetch + Blob（不再 `<a href>`） | **成立**（可用性见待确认 3） | `frontend/src/api.ts:136-153`、`frontend/src/components/FileRow.vue:15-27` |
| 五道并发闸门互相独立 | **部分成立**：精确路径下确实独立，但 `/api/events/` 可绕过事件闸门（F3） | `scripts/probe-gates.sh`、`api/server.go:117-132` |
| UDP 必须带 payload | **成立** | `core/config.go:674-687`；a2s/q3 自带查询且拒绝额外 `payload`；`parsePayload` 拒绝空/非法 hex |
| A2S / Q3 回包校验 | **成立**（按 DESIGN 的"认前缀"标准） | `core/probe.go:241-272`：A2S 认 `0xFFFFFFFF`+`I/A/E` 或分片 `0xFFFFFFFE`；Q3 认 `0xFFFFFFFF/0xFFFFFFFE` 前缀；非本协议回包判 down |
| `Monitor.Stream` 不因单轮探测失败退出 | **成立** | `app/monitor.go:233-266`；`TestStreamLoopKeepsGoingAfterProbeFailure`；`probe-reload.sh` 里失败后进程仍在服务 |
| 配置加载 fail-fast | **大部分成立**：类型/范围/密钥形状都有校验，但 `STREAM_INTERVAL` 的 NaN/Inf/溢出与 `probe.timeout` 的 0 能静默通过（F4、F5） | `scripts/probe-stream-interval-env.sh`、`scripts/probe-probe-timeout-zero.sh` |
| SSE 重连不再自我复制 | **代码层面成立**（`frontend/src/sse.ts:22-73` 每个 run 自带 `alive`），未做浏览器长跑验证 | 代码审查 |

---

## 构建 / 测试（实际执行）

```bash
cd /home/user/repos/hm-service/scope-status
go vet ./...                       # exit 0，无输出
go test ./... -count=1             # exit 0
# ok  github.com/hm-service/scope-status
# ok  github.com/hm-service/scope-status/api        5.526s
# ok  github.com/hm-service/scope-status/app        1.410s
# ok  github.com/hm-service/scope-status/core       0.387s
# ok  github.com/hm-service/scope-status/httpx      0.003s
# ok  github.com/hm-service/scope-status/i18n       0.005s
# ok  github.com/hm-service/scope-status/store      0.010s
# ?   github.com/hm-service/scope-status/internal/testenv  [no test files]

cd frontend && node_modules/.bin/vue-tsc -b   # exit 0（注意：在仓库根执行 node_modules/.bin/vue-tsc 不存在，需在 frontend/ 下跑）
```

探针（全部在 `scripts/` 下，可直接重跑）：

| 脚本 | 结果 |
| --- | --- |
| `probe-gates.sh` | 复现：`/api/events/` 占业务闸门，`/api/stats` 排队超时；对照 `/api/events` 正常 |
| `probe-message-race.sh` | 复现：删除成功后又复活（`liveMessages=1`），附件已被 Sweep 删除 |
| `probe-message-race-api.sh 30 1` | 复现（API 1 MiB 正文上限内）：30 轮 27 次复活、1 次附件同时丢失；`probe-message-race-api.sh 30 0`（同时起跑）本轮 1 次命中、上一次 0 次，说明命中率随时序而变、但不需要人为偏移也能撞上 |
| `probe-reload.sh` | 复现：reload 回 500，但新配置已生效（peers=2）、旧快照清空（`/api/status` 204、`?force=1` 500）；进程仍活着（循环没退出） |
| `probe-stream-interval-env.sh` | 复现：`NaN` / `Inf` / `1e10` 均启动成功且 SSE 循环静默不启动；`abc` 正常拒绝启动 |
| `probe-sweep-orphan-upload.sh` | 复现：上传的附件被无关删除的 Sweep 回收，随后建消息 400 |
| `probe-probe-timeout-zero.sh` | 复现：`probe.timeout: 0` + 不响应目标，`/api/status?force=1` 5 秒挂住；`0.2` 对照 209 ms 返回 |
| `probe-shutdown-sse.sh` | 复现：开着 SSE 时 SIGTERM → 5009 ms、exit 254、`server shutdown forced` |

## 未验证项

- 前端行为（F8、F9、F12）只做了代码静态推演，没有跑浏览器 E2E/Playwright，也没有在真实反代（nginx/Caddy）后面验证 SSE 与 pending 行为。
- 没有跑 `go test -race ./...`；F1 是逻辑竞态（无共享内存读写），`-race` 也不会捕获，故未作为证据。
- 没有跑 `pnpm build`（`vite build`）与镜像构建，只做了 `vue-tsc` 类型检查；Dockerfile / CI 里的 node 26.13.0、pnpm 12.18.0、golang 1.27.4 镜像未实际拉取。
- 没有做容量/压测（F13 的内存与 `/readyz` 成本只有静态估算）。
- 没有在 k8s/podman 下验证优雅退出与 PVC 权限。
- 未接触生产环境；所有探针都在临时目录 + 随机端口上运行，未使用任何真实 token。