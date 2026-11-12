# Response — scope-status @ bdbf312

- **对应审查**：`code-reviews/2026-11-13T01-18-54+0800_bdbf31/REVIEW.md`（被审 commit `bdbf31246da9057b137dd90b3bb3d1d0f6563d4d`）
- **回应时间**：2026-11-13 01:52 +0800（Asia/Shanghai）
- **结论**：14 条里 **11 条已修**（含 P0/P1/P2 各若干），**3 条未修**并说明原因（F6、F7、F13，都需要改 `DESIGN.md` 或做产品决策）。另修 1 条审查没列的问题（会话 token 的 base64 非严格解码）。

| 编号 | 严重度 | 状态 |
| --- | --- | --- |
| F1 | P0 | 已修 |
| F2 | P1 | 已修 |
| F3 | P2 | 已修 |
| F4 | P2 | 已修 |
| F5 | P2 | 已修 |
| F6 | P2 | **未修**（与 `DESIGN.md:184` 冲突，待定） |
| F7 | P2 | **未修**（`DESIGN.md:281` 写的就是全池清扫，待定） |
| F8 | P2 | 已修 |
| F9 | P3 | 已修 |
| F10 | P3 | 已修 |
| F11 | P3 | 已修 |
| F12 | P3 | 已修 |
| F13 | P3 | **未修**（要不要设上限是产品决策） |
| F14 | P3 | 已修 |

## 逐条

### F1（P0）消息 Update / Delete 无锁 —— 已修

- **位置**：`store/messages.go`
- **做法**：
  - `MessageStore` 增加 `mu sync.Mutex`，把「读整份 json → 改 → 整份写回」以及紧随其后的附件回收整段收进临界区；`Create` / `Update` / `Delete` / `Sweep` 都在锁内。
  - `Delete` 内部调用私有 `sweep()`（`Sweep()` 是对外版本，自己加锁），避免自锁。
  - 写文件的临时文件名加 6 字节随机后缀，同一 id 的两次写不再落到同一个路径互相截断。
- **验证**：
  - `scripts/probe-message-race.sh` 重跑：`list after both: liveMessages=0`（修前是 `1`，删除被复活）。
  - `scripts/probe-message-race-api.sh 30 1` 重跑：`attempts=30 resurrected=0`。
  - 新增 `store/messages_test.go:TestConcurrentUpdateAndDeleteKeepsDeletion`（并发 `Update` + `Delete`，断言结果必为已删除），`-race -count=10` 通过。
- **残留**：进程内锁。多副本同时写同一个 DATA_DIR 时仍无跨进程互斥；当前部署是单副本 + 单 PVC，`DESIGN.md` 也未承诺多写者。若将来要多副本写，需要文件锁或版本号，这属于设计变更，未擅自做。

### F2（P1）reload 探测失败时新配置已生效、旧快照被清空 —— 已修

- **位置**：`app/monitor.go`（`Reload`）、`api/status.go`（`handleReload`）
- **做法**：`Reload` 改成「解析新配置 → 用新配置先探一轮 → 都成功才一次性提交 `source` + `snapshot` + `reloadCount` 并广播」。失败时旧配置与旧快照原样保留，不留「回 500 但状态已变」的中间态。返回三个值 `(source, snapshot, error)`，`handleReload` 不再二次 `Refresh()`（顺带省掉一轮重复探测）。
- **并发**：`Reload` 的探测与 `Refresh` 的 single-flight 串行，避免旧配置的一轮在换配置之后才把快照写回去。
- **验证**：
  - `scripts/probe-reload.sh` 重跑：`POST /api/reload -> HTTP 500`，`peers` 仍是 `1`（旧配置），`GET /api/status` 仍是 `200`（旧快照还在）。
  - 真实二进制手测（`probe.method: tcp` + 一条 `nope.invalid`）：`{"detail":"nope.invalid 解析失败"}` / `HTTP 500`，随后 `/api/stats` 仍 `peers=4`、`/api/status` 仍 `200`。
  - 新增 `app/monitor_test.go:TestReloadKeepsOldStateWhenProbeFails`，断言旧配置、旧快照（`GeneratedAt` 不变）、`reloadCount` 不增。
  - 原 `TestReload` 里「换了配置要丢掉旧快照」的断言按新语义改成「成功就该拿到新配置的快照」。

### F3（P2）`/api/events/` 绕过事件闸门 —— 已修

- **位置**：`api/server.go:117`（`gate`）
- **做法**：`gate()` 先 `strings.TrimRight(path, "/")` 再查表，与路由层 `matchSegments` 的 Trim 行为对齐；空串（路径就是 `/`）落到静态闸门。
- **验证**：`scripts/probe-gates.sh` 重跑，`/api/events/` 建立后 `GET /api/stats` 立刻 `200 0.0006s`（修前是 2 秒超时 `000`）。新增 `api/concurrency_test.go:TestEventGateIgnoresTrailingSlash`。

### F4（P2）`STREAM_INTERVAL=NaN/Inf/溢出` 静默关掉 SSE 循环 —— 已修

- **位置**：`main.go`（`envFloat`、`loadEnv`）
- **做法**：`envFloat` 拒绝 `NaN` 与 `±Inf`；`STREAM_INTERVAL` 限定 `0 ≤ v ≤ 86400`（新增常量 `maxStreamSeconds`），换算成 `time.Duration` 前就把溢出值挡在门外。
- **验证**：新增 `main_test.go:TestLoadEnvStreamInterval`，覆盖默认值、`0`、`60`，以及 `abc` / `NaN` / `nan` / `Inf` / `+Inf` / `-Inf` / `-1` / `1e10` 全部拒绝启动。`scripts/probe-stream-interval-env.sh` 重跑：三种值都变成 `exit 1`。

### F5（P2）`probe.timeout: 0` 等于不设超时，能卡死整轮探测 —— 已修

- **位置**：`core/config.go`（`parseProbe`、`parseLinks`）
- **做法**：`probe.timeout` 要求 `0 < v ≤ 60`（新增 `maxProbeTimeoutSeconds`）；`probe.port` 复用服务端口的 1–65535 校验；`links.expect_status` 限定 100–599。新增三个文案键 `KeyConfigProbeTimeoutRange` / `KeyConfigProbePortRange` / `KeyConfigLinkExpectStatusRange`（中英各一条），不复用「必须是数字」的键——报错要说清是范围问题。
- **验证**：`core/config_test.go` 错误表加 7 个用例（timeout 0 / 负数 / 61，port 0 / 70000，expect_status 99 / 600）。`scripts/probe-probe-timeout-zero.sh` 重跑：`timeout: 0` 现在启动即失败。

### F6（P2）一条地址 DNS 失败丢弃整轮结果 —— 未修

- **原因**：这条与 `DESIGN.md:184-185` 直接冲突：

  > `status` 只有 `up` / `down`；判不出结论（没有 `ping`、域名解析不了）时整个探测报错，不编状态。

  改成「折成一个 down」等于替设计做了相反的取舍（「不编状态」→「编一个 down + DNS 文案」），而 `DESIGN.md` 由你定，我不擅自改。我把改动做出来跑通后又回退了，代码保持原样。
- **给下一次的候选方案**（需要你点头改 `DESIGN.md` 才能落）：探测层把 DNS 失败折成该条目的 `down + KeyDetailDNSFailed`，只有配置/内部错误才让整轮失败；ICMP 方法下本来就是这个行为（`core/probe.go:93-105`），两种方法因此一致。
- **顺带**：`source.example.yaml:62` 的注释里「会把 SSE 循环直接停掉」是 `b9a872f` 之前的旧行为（现在循环不退出、只是这一轮不出快照），已按当前实现改写。

### F7（P2）刚上传未被引用的附件会被无关删除的 Sweep 回收 —— 未修

- **原因**：`DESIGN.md:281-282` 把全池清扫写成了设计：

  > 随后扫一遍所有 `message*.json`，只回收不再被任何**未删除**消息引用的池内文件。回收只由删除触发，孤儿附件（传了又没发）等下一次删除才会被扫掉。

  改成「只回收这条被删消息自己的附件」会让孤儿附件永远留在盘上，是有意的取舍被反过来了。三条路都不该由我单方面选：
  1. 保持现状（按设计），接受这个窗口；
  2. 只回收被删消息自己的引用 → 孤儿附件永不回收，磁盘泄漏；
  3. 上传记 pending 时间戳，清扫只收超过 N 分钟仍无引用的文件 → 引入一个魔数。
- **建议**：选 1 还是 3 请你定；选 3 我可以顺手把 `DESIGN.md` 那两行一并改掉（需要你授权）。

### F8（P2）前端静默检测误判并反复重连 —— 已修

- **位置**：`frontend/src/sse.ts`、`frontend/src/store.ts`
- **做法**：
  - `StreamHandlers` 增加 `onActivity()`，`readFrames` 每读到一段字节就回调一次——保活注释帧（`: ping`）也算「连接活着」。
  - 静默阈值改成 `max(45s, 3 × streamInterval)`（`streamSilenceMs()`），不再写死 30 秒；`STREAM_INTERVAL=60` 下阈值是 180 秒。
  - 新增 `state.streamSilent`：收到任意活动就把「静默」那条错误摘掉，不再一直挂在页脚。
- **验证**：`vue-tsc -b` 通过；真浏览器打开看板（`STREAM_INTERVAL=2`）页面正常、SSE 显示 streaming、console 无报错。未做「阈值到点」的时长验证（要真等 3 分钟以上），属于未自动验证项。

### F9（P3）`STREAM_INTERVAL=0` 时前端从不主动探一次 —— 已修

- **位置**：`frontend/src/store.ts`
- **做法**：`onHello` 里发现 `streamInterval <= 0` 且还没快照，就走一次 `fetchOnDemandSnapshot()`（`GET /api/status`，走服务端的按需探测）；失败挂到页脚，不弹「已刷新」这类提示（这不是用户点的按钮）。
- **验证**：代码路径与 `vue-tsc` 通过；未用 `STREAM_INTERVAL=0` 起真浏览器跑（环境里没留证据，列在未验证项）。

### F10（P3）有 SSE 在线时 SIGTERM 每次跑满 5 秒并强关 —— 已修

- **位置**：`api/server.go`（`Options.Stopping`）、`api/events.go`、`main.go`
- **做法**：新增 `Options.Stopping <-chan struct{}`；`handleEvents` 的 select 多一个 `<-s.opts.Stopping` 分支，收到就主动收尾。`main.go` 在 `server.Shutdown` 之前 `close(stopping)`。只影响 SSE，不等同于取消在途的普通请求（那些仍由 `Shutdown` 的宽限期保证）。
- **验证**：`scripts/probe-shutdown-sse.sh` 重跑：`退出码 0`、`从 SIGTERM 到退出 8ms`、日志是 `server shutdown` 而非 `server shutdown forced`（修前 5009ms / exit 254）。

### F11（P3）变更已落盘后 publish/Sweep 出错仍回 500 —— 已修

- **位置**：`app/messages.go`、`store/messages.go`
- **做法**：
  - `MessageService` 在 `Create/Update/Delete` 成功后调 `publishQuietly()`：推帧失败只记一条 `messages broadcast failed` 日志，不把已成功的写操作变成 500。
  - `MessageStore.Delete` 里删除标志写成功之后的清扫失败也只记日志（`message sweep failed`），返回成功。
- **验证**：`go test ./...` 全绿；现有用例覆盖「目录里有坏文件」的读路径（`TestBrokenMessageFiles`）。「坏文件存在时创建消息仍返回成功」这一条没有单独加用例——构造「写成功但广播读失败」的时序要额外注入，未做，列在未验证项。

### F12（P3）`connect_cmd` 复制按钮在明文 HTTP 下必然失败 —— 已修

- **位置**：`frontend/src/components/CopyButton.vue`
- **做法**：没有 `navigator.clipboard`（非安全上下文）时退回「临时 textarea + select + `document.execCommand('copy')`」；两条都失败才提示复制失败。
- **验证**：`vue-tsc -b` 通过；未在明文 HTTP 的真浏览器里点一次（浏览器E2E只做了主页面渲染），列在未验证项。

### F13（P3）消息数量/正文没有上限，每次变更全量读盘并广播 —— 未修

- **原因**：`core/messages.go:13` 明确写着「消息条数、正文长度、附件个数都不设限」，这是当前设计。要不要加上限、上限多少、超了是拒绝还是分页，是产品决策；同时 `DESIGN.md:313` 与这句「不设限」的表述本身也需要一起改，我不擅自改 `DESIGN.md`。
- **可随时落地的部分**（等你定）：给条数/正文设上限；SSE 推增量而不是每帧整份列表；`List` 分页。这条改动量最大，建议单开一轮。

### F14（P3）死代码与重复实现 —— 已修

- **位置**：`core/config.go`、`core/snapshot_test.go`、`frontend/src/store.ts`、`frontend/src/types.ts`
- **做法**：删掉 `Category.Label` 字段与 `Render` 里那条永不命中的分支（`Categories` 只由 `usedCategories`/`builtinCategory` 产生，配置里没有 `label` 可读）；前端 `resolveGroups` 只认后端给的 `payload.groups`，删掉按 `categories` 兜底分组的那套逻辑与 `CategoryInfo` 类型。分组规则从此只有 `core.groupBy` 一个来源。

## 审查没列、这次一并修的

### [P2] 会话 token 的 base64 解码不严格，改一个字符也能解出同一段签名

- **位置**：`core/session.go`
- **怎么发现的**：改完 F2 跑全量测试时 `TestSessionIssueAndValid` 偶发失败（`签名被改 不该认`）。原因是 base64url 末字符只用到 4 位，`flipLast` 把最后一个字符 `X` 改成 `A` 时，解码得到的 32 字节可能一模一样——`base64.RawURLEncoding.DecodeString` 默认不校验「多余的位必须是 0」，于是签名照旧通过。同一个签名因此有多种字符串写法。
- **做法**：解码统一走 `base64.RawURLEncoding.Strict()`（新增包级 `sessionEncoding`），非规范编码直接拒。
- **验证**：`go test ./core/ -run TestSession -count=30` 全绿；严格解码后 `flipLast` 的任意改动都会落到不同字节或解码失败。

## 构建与测试（实际执行）

```bash
cd /home/user/repos/hm-service/scope-status
just check                        # go vet + go test ./... + frontend pnpm install/typecheck，全绿
just build                        # vue-tsc + vite build + linux/amd64 & linux/arm64 两个静态二进制，成功
go test ./... -count=1            # 全部 ok（main / api / app / core / httpx / i18n / store）
go test -race ./... -count=1      # 全部 ok
go test ./... -cover -count=1     # main 50.0 / api 84.0 / app 75.1 / core 84.5 / httpx 64.3 / i18n 87.1 / store 73.7
go test ./core/ -run TestSession -count=30   # 全绿（严格解码后不再有偶发失败）
```

- **测试函数数**：修前 121 → 修后 125（新增 `TestEventGateIgnoresTrailingSlash`、`TestLoadEnvStreamInterval`、`TestConcurrentUpdateAndDeleteKeepsDeletion`、`TestReloadKeepsOldStateWhenProbeFails`），另有 `core/config_test.go` 新增 7 个表驱动子用例。
- **覆盖率变化**：`README.md:272-278` 的表格已按实测更新（main 46.0→50.0、core 84.4→84.5、store 72.7→73.7、app 74.7→75.1、api 83.8→84.0）。
- **真实二进制手测**（`HOST=127.0.0.1 PORT=18081 DATA_DIR=.data_temp STREAM_INTERVAL=2`，配置由 `source.example.yaml` 派生）：
  - `/livez` 200、`/readyz?token=…` 200、`/` 200；
  - `POST /api/auth` 换到会话（88 字符），带会话 `/api/stats` 200；
  - `/api/events/`（尾斜杠）能连，同时 `/api/stats` 200（50µs）；
  - `POST /api/reload` 正常配置 200；改成一条解析不了的地址后 500 + `peers` 仍 4 + `/api/status` 仍 200；
  - 浏览器（headless Chrome）打开看板：拓扑、3 个分组、SSE streaming、无 console 报错；
  - SIGTERM 退出干净，日志 `server shutdown`。

## 未自动验证项

- F8 的静默阈值要等 `max(45s, 3×interval)` 才触发，没有做长跑计时验证。
- F9（`STREAM_INTERVAL=0` 开机自动探一次）与 F12（明文 HTTP 下的复制回退）只有类型检查 + 代码路径确认，没有在真浏览器手动点一次。
- F11 的「写成功但广播失败」时序没有专门注入用例。
- 镜像构建（Dockerfile / CI）没有在本机跑，多架构构建只用 `just build` 的交叉编译验证过。
- 上一轮 review 列出的「待确认」1–6 这次没有动（除第 6 条随 F14 一并消失：前端 `categories` 字段已删）。