# DESIGN.md

Scope Status 的设计：内网 WireGuard 连通状态看板。单文件静态二进制，容器里跑，数据目录即全部状态。

## 1. 部署

| 环境变量                 | 默认值                 | 说明                                                     |
| ------------------------ | ---------------------- | -------------------------------------------------------- |
| `DATA_DIR`               | `/data`                | 数据目录：配置 `source.yaml` 与消息 `messages/` 都在这里 |
| `TZ`                     | `Asia/Shanghai`        | 时区；影响日志时间戳与消息文件名                         |
| `HOST`                   | `127.0.0.1`            | 监听地址                                                 |
| `PORT`                   | `8080`                 | 监听端口                                                 |
| `AUTH_TOKEN`             | 无                     | **必填**，整个站点的 Bearer token；为空（或只有空白）拒绝启动。读完立刻 Argon2id 散列，进程里只留摘要 |
| `READYZ_TOKEN`           | 空                         | 非空时 `/readyz` 必须带 `?token=`；16–128 位小写字母/数字/下划线，写错拒绝启动 |
| `TRUSTED_PROXIES`        | `127.0.0.1/32,::1/128` | 可信反代来源，决定真实客户端 IP 怎么取                   |
| `STREAM_INTERVAL`        | `5`                    | SSE 循环探测间隔秒数；`0` 表示不起循环（只按需探测）     |
| `MAX_API_CONCURRENCY`    | `128`                  | API 路由（含 `/livez` `/readyz`）并发上限                |
| `MAX_STATIC_CONCURRENCY` | `64`                   | 静态文件并发上限                                         |
| `GOMEMLIMIT`             | `128MiB`               | Go GC 阈值，由运行时自己读                               |
| `MAX_UPLOAD_BYTES`       | `1073741824`           | 单个附件上限（1 GiB）                                    |

- 配置与状态同目录，配置路径写死为 `<DATA_DIR>/source.yaml`：容器把配置当 ConfigMap 挂在数据目录里，
  状态挂 PVC；挂一个卷就是应用的全部。
- 启动即失败：`AUTH_TOKEN` 没设 / 配置文件不存在 / 解析不通过 / 数据目录建不出来，进程直接报错退出，不做任何降级。
- 配置路径没有第二个来源，也没有环境变量能改它；改 `DATA_DIR` 就等于改配置路径，界面上与 `/api/health` 里显示的永远是当前生效的那一份。
- Kube 部署时 `<DATA_DIR>/source.yaml` 由人手动放进卷（`podman cp` 或宿主目录型 PV），清单里不挂它。
- 工具链版本在 `go.mod`、`Dockerfile`、CI、`frontend/package.json` 四处对齐（当前 go 1.27.2 / node 26.7.0 / pnpm 12.13.0）。

## 2. 页面

四个标签页，顶栏常驻：标题、SSE 状态、语言切换、主题切换、刷新、重载配置（窄屏收进 `⋯` 菜单）。
另有一个独立入口 `/auth`：认证页。

| 标签页 | 内容                                                               |
| ------ | ------------------------------------------------------------------ |
| 状态   | 内网条目：一行一个，地址、延迟、连通性；按 `category` 分组，可折叠 |
| 服务   | 游戏服 / 站点后端：按 `protocol` 探测；`connect_cmd` 可复制        |
| 网站   | 链接：站点图标 + 名字可点开；显示 HTTP 状态                        |
| 消息   | 文本与附件；不分类型，进站的人都能看；按时间戳倒序                 |

- 每页顶部一条概况栏（总数 / 在线 / 离线，消息页是总数 / 最近消息），四个页高度一致，
  切换标签不抖。
- 进站先查 token：本地没有就 `location.replace('/auth')`；`/auth` 拿 token 换一次 `POST /api/auth`，
  通过了存进 `localStorage` 再回首页。认证步骤至少 1 秒，这期间按钮是「校验中…」（自动校验时也一样）。
  主站与认证页共用一套主题与语言设置。
- 主题：默认浅色，可切深色，记在 `localStorage`；≤560px 走卡片式布局。
- 语言：默认按 `navigator.language`（`zh*` → 简体中文，其余 → English），顶栏可切换，记在
  `localStorage`；每次 `fetch` 带 `Accept-Language`，服务端错误文案跟着一起切。

## 3. 接口

除附件下载外一律 `application/json`；错误体统一是 `{"detail": "…"}`（按 `Accept-Language` 渲染好的文案），
`/api/*` 下的 404 / 405 也是这个形状，405 带 `Allow`（`GET` 按 Go 的语义写成 `GET, HEAD`）。

**鉴权**：除 `POST /api/auth` 外，所有 `/api/*` 都要 `Authorization: Bearer <AUTH_TOKEN>`。
没带、格式不对、token 不对都是 `401` + `WWW-Authenticate: Bearer`，`detail` 按语言渲染；
比较用恒定时间，token 只从环境变量来，盘上不落。静态资源（含 `/auth` 页）与 `/livez` `/readyz` 不受影响：
浏览器要能先打开认证页，kubelet 要能直接探。

`POST /api/auth` 是**认证步骤**：至少花 1 秒（成功与失败一样），别让人拿它当高速试探口；
请求级的 Bearer 校验只走一次 Argon2id（约 30 ms），不再叠这个下限——否则每个 API 调用、每次 SSE 重连都要等一秒。

响应头默认带 `Content-Language` + `Vary: Accept-Language`，以及 `X-Content-Type-Options: nosniff`、
`Referrer-Policy: no-referrer`、`X-Frame-Options: DENY`。

| 方法         | 路径                                    | 说明                                                         |
| ------------ | --------------------------------------- | ------------------------------------------------------------ |
| `POST`       | `/api/auth`                             | 请求体里的 `token` 换 `{ok:true}`；认证步骤至少 1 秒，不对 `401`（唯一免鉴权的 /api 口） |
| `GET`        | `/api/status`                           | 当前共享快照；还没有快照时 `204`                             |
| `GET`        | `/api/status?force=1`                   | 强制探一次再回                                               |
| `GET`        | `/api/events`                           | SSE：先补一帧 `messages`，再推 `snapshot` / `messages`       |
| `POST` `GET` | `/api/reload`                           | 重读 `<DATA_DIR>/source.yaml`，成功则立刻探一次              |
| `GET`        | `/api/health`                           | 运行状态与计数                                               |
| `GET`        | `/api/messages`                         | 未删除消息列表（每条都是完整内容）                           |
| `POST`       | `/api/messages`                         | 新建；`201`                                                  |
| `GET`        | `/api/messages/{id}`                    | 看一条                                                       |
| `PATCH`      | `/api/messages/{id}`                    | 改正文 / 附件                                                |
| `POST`       | `/api/messages/{id}/delete`             | 打删除标志位并回收无人引用的附件                             |
| `PUT`        | `/api/messages/files?name=<文件名>`     | 附件上传：请求体就是裸文件流，不拼 multipart                 |
| `GET`        | `/api/messages/files/{sha256}?id=`      | 下载附件；附件池共享，必须指明是哪个消息引用的它             |
| `GET`        | `/livez`                                | 进程活着就 `200`，不记日志                                   |
| `GET`        | `/readyz?token=`                        | 依赖就绪（配置已载入、数据目录可写）才 `200`，记一条审计日志 |
| `GET`        | `/`、`/auth` 及其余路径                 | 内嵌前端产物；`/auth` 映射到 `auth.html`；未知 `/api/*` 回 `404` JSON |

`POST /api/reload` 成功时回 `{ok, message, peers, services, links, reloadCount, sourcePath, snapshot}`：
`message` 是服务端按 `Accept-Language` 渲染好的一句提示，`snapshot` 就是下面那份快照（`reused: false`）。
失败时回 `500`，`detail` 就是配置校验的报错原文，旧配置继续生效。

服务端渲染的文案（探测 `detail`、错误正文）按**请求**（SSE 则是**连接建立时**）的 `Accept-Language` 决定；
前端的界面文案走自己的表，切语言不必重连，但服务端渲染的那部分要等下一次请求 / 重连才换语言。

### 3.1. 快照

```json
{
  "title": "Scope Status",
  "method": "icmp",
  "generatedAt": "2026-10-12T23:35:03+08:00",
  "durationMs": 412.5,
  "reloadable": true,
  "reloadCount": 0,
  "streamInterval": 5,
  "summary": { "total": 3, "online": 2, "offline": 1 },
  "groups": [
    {
      "id": "friend", "label": "朋友", "order": 10, "total": 1, "online": 1,
      "peers": [
        {
          "name": "小明", "note": null, "kind": "friend", "online": true, "latencyMs": 12.4,
          "addresses": [
            { "address": "10.0.0.2", "status": "up", "latencyMs": 12.4, "detail": null, "method": "icmp" }
          ],
          "properties": { "面板账号": "example-pass" }
        }
      ]
    }
  ],
  "peers": [
    {
      "name": "小明", "note": null, "kind": "friend", "online": true, "latencyMs": 12.4,
      "addresses": [], "properties": { "面板账号": "example-pass" }
    },
    {
      "name": "云服务器-0", "note": null, "kind": "cloud", "online": true, "latencyMs": 7.1,
      "core": true, "private": ["10.0.0.1"], "public": ["203.0.113.10"], "wgPeers": ["云服务器-1"],
      "addresses": [], "properties": {}
    }
  ],
  "services": [
    {
      "name": "MC 服务器", "host": "10.0.0.9", "port": 25565, "address": "10.0.0.9:25565",
      "protocol": "a2s", "connectCmd": "mc 10.0.0.9", "category": "server",
      "properties": {}, "online": true, "latencyMs": 8.1,
      "detail": "A2S_INFO 有回应", "method": "a2s", "status": "up"
    }
  ],
  "servicesGroups": [],
  "servicesSummary": { "total": 1, "online": 1, "offline": 0 },
  "links": [
    {
      "name": "面板", "url": "https://panel.example.com/", "probeUrl": "https://panel.example.com/",
      "expectStatus": 200, "properties": {}, "online": true, "latencyMs": 88.2,
      "detail": "HTTP 200", "status": "up", "faviconUrl": "https://panel.example.com/favicon.ico"
    }
  ],
  "linksSummary": { "total": 1, "online": 1, "offline": 0 }
}
```

- `peers` 是扁平的全量列表（`peers:` 块和 `cloud_servers:` 块解析完并在一起），`groups` 是按分好组的
  同一批对象；`services` / `servicesGroups` 同理。
- `kind` 只有三个值：`friend` / `server`（配置里写）/ `cloud`（来自 `cloud_servers` 块）。
  `core` / `private` / `public` / `wgPeers` 只有 `kind = cloud` 才有，其余条目不会出现这些键。
  `wgPeers` 是服务端按 `core` 推出来的（主干 ↔ 其余每一台，两边都给对端名字），配置里不用写。
- 网站图标（`faviconUrl`）**异步**抓：`For` 只读缓存，未命中就丢给后台（同 origin 只抓一次），
  所以某一帧里它可能是 `null`，下一帧补上——抓图标最慢要十几秒，不能挂在探测的关键路径上。
- `status` 只有 `up` / `down`；判不出结论（没有 `ping`、域名解析不了）时整个探测报错，
  不编状态：`/api/status?force=1` 会回 `500`（`detail` 是原因），SSE 循环则记一条带文案键的日志后终止。

### 3.2. SSE

```
event: messages
data: {"messages":[…],"summary":{"total":1}}

event: snapshot
data: {…快照，多一个 "reused": false…}

: ping
```

- 连上先补一帧 `messages`（当前列表），再补一帧 `snapshot`（有快照时，`reused: true`）
  或 `hello`（`{"streamInterval":5}`）。
- 前端不用 `EventSource`（它没法带 `Authorization` 头，token 只能塞 URL 里），而是用
  `fetch` + `ReadableStream` 自己分帧：空行分帧、`field: value` 取字段、注释帧丢弃，断线 3 秒后重连。
- 之后每轮探测完成推一帧 `snapshot`；消息有任何变更推一帧 `messages`。
- 15 秒没事件就发一个注释帧 `: ping` 保活；队列满时丢最旧的一帧（订阅者掉队不拖累服务端）。
- 长期没有订阅者时探测降到每分钟一次。

## 4. 存储

### 4.1. `<DATA_DIR>/source.yaml`

这份文件是本机手写的，里面通常有密码，所以整个数据目录都不入库；入库的只有 `source.example.yaml` 这份示例。
路径固定成 `DATA_DIR` 根下的 `source.yaml`，名字不再是可配置项。

顶层 mapping，五块：`cloud_servers` / `peers` / `services` / `links`（`probe`、`title` 可选）。
顶层直接写数组等价于 `peers`。字符串字段一律要求非空，类型不对就报错，不猜、不转换；字段名只有表里这一种
写法，不认别名（历史别名一律删掉，不做向后兼容）。分组不用声明：`peers` 按 `kind` 走内置名字
（朋友 / 服务器），`cloud_servers` 固定进「云服务器」，`services` 用 `category`（没写就是内置「服务」）。

| 字段            | 适用     | 约束                                                                       |
| --------------- | -------- | -------------------------------------------------------------------------- |
| `name`          | peers / cloud_servers / services / links | 非空字符串，同一页内不要求唯一                |
| `kind`          | peers    | **必填**，`friend` 或 `server`；决定图标与分组                             |
| `addresses`     | peers    | 非空数组，每项 IP 或域名（可带 `:端口`），去重                             |
| `note`          | peers / cloud_servers | 备注                                                             |
| `properties`    | 四者     | 名称 → 值的映射，按书写顺序展示；值可以是数字、布尔                        |
| `private`       | cloud_servers | 隧道内地址数组，可写多个；和 `public` 至少要有一个                      |
| `public`        | cloud_servers | 公网入口地址数组，可写多个                                             |
| `core`          | cloud_servers | 布尔；`true` = WG 主干，整个文件最多一个；WG 链路按它推：主干 ↔ 其余每一台 |
| `category`      | services | 分组名（转小写、空格转 `-`）；不写就进内置「服务」组                       |
| `host` `port`   | services | `host` 非空；`port` 1–65535                                                |
| `protocol`      | services | `udp` / `tcp` / `a2s`                                                      |
| `connect_cmd`   | services | 可复制的连接命令                                                           |
| `url`           | links    | `http(s)://` 开头的完整地址                                                |
| `probe_url`     | links    | 探测地址；不写就探 `url`                                                   |
| `expect_status` | links    | 视为「通」的 HTTP 状态码，默认 `200`                                       |
| `probe`         | 顶层     | `method`（`icmp`/`tcp`/`http`）、`timeout`、`port`、`concurrency`、`count` |

`cloud_servers` 里的条目解析完会**并进 `peers` 列表**（`kind = cloud`），所以探测、分组、摘要、
表格、拓扑图都走同一条路；`private` + `public` 两个数组拼起来就是它的待探测地址。

### 4.2. 拓扑图

状态页最上面是一张从 `peers` 现算的 SVG（前端算，后端不掺和），分两层：

- **中继基建**：一个容器，里面只摆 `kind = cloud` 的条目（也就是 `cloud_servers` 块）。
  相连的两台之间牵金色双线：链路由后端按 `core` 推好（主干 ↔ 其余每一台，两边都给 `wgPeers`），
  前端只做无向去重，不臆造。
- **接入节点**：其余条目用双箭头接进容器（服务器类的接上边缘、朋友等接下边缘），
  箭头颜色 = 该条目各地址的汇总状态（全通绿、部分通金、全断红虚线）。

`core: true` 的那台会画得更大：虚线底色板 + 更粗的描边 + 「核心中继」徽标 + 一行 `WG 主干 → …`。
类型图标按 `kind` 走（人 / 机架 / 云朵），子网配色按 /24 前缀分配。
配色与文案都跟着主题和语言走。

图放在一个**画布**里，不跟着正文列宽把字缩小：默认按「适应宽度」显示（约 81%），
滚轮或 `+` / `−` 缩放（50%–400%），按住拖动平移，双击或「复位」回到适应宽度；
滚轮已经缩到最小还继续往下滚时不再拦截，让页面正常滚动。窄屏最少按 55% 画，宁可横向拖也不缩成蚂蚁。

### 4.3. `DATA_DIR/messages`

盘上只有两样东西，不分消息类型：

- `messages/message-yyyy-MM-dd-HH-mm-ss-ms(4)-随机6位.json` —— 一条消息一个文件。
  创建时间只在文件名里（唯一真相来源），消息本体按原样存：

  ```json
  {
    "text": "正文",
    "updatedAt": 1760273703085,
    "attachments": [{ "sha256": "<64 hex>", "name": "豆包.png", "size": 3872014 }],
    "isDeleted": false
  }
  ```

  消息不分类型，盘上没有 `kind` / 口令字段。读到不合规的文件（缺 `text` / `isDeleted`、类型不对、
  id 对不上文件名）就是 `500`，不修复、不跳过；不认识的字段忽略。

- `messages/files/<sha256>` —— 附件池，按内容寻址，同内容天然一份。上传时边写边算 sha256，
  写完按内容改名；池子里已有就丢掉刚写的临时文件。

删除用 `isDeleted` 标志位：json 原地保留，随后扫一遍所有 `message*.json`，只回收不再被任何
**未删除**消息引用的池内文件。回收只由删除触发，孤儿附件（传了又没发）等下一次删除才会被扫掉。

## 5. 安全模型

- **一道门：`AUTH_TOKEN`**。除 `POST /api/auth` 外的每个 `/api/*` 请求都要
  `Authorization: Bearer <token>`。空值拒绝启动，不提供默认值——没设就等于把接口敞开。
- **内存里只有摘要**：启动时读进来的 token 立刻过 Argon2id（`m=19 MiB, t=2, p=1`，盐随机，PHC 串），
  明文那份随 `loadEnv` 返回一起不可达，`api.Options` 里只有摘要；`os.Unsetenv("AUTH_TOKEN")` 顺手清掉环境变量，
  `ping` 这类子进程继承不到。每个请求把请求头里那份 token 用同一个盐再散一遍，`crypto/subtle` 恒定时间比较
  （一次约 30 ms、19 MiB，同并发最多 4 个哈希在算）。
  同机同用户的进程仍能从 `/proc/<pid>/environ` 读到原始 exec 环境块，真要躲开这一层就把 token 放 Secret / 一次性文件注入。
- **认证步骤至少 1 秒**：`POST /api/auth` 无论成功失败都补足到 1 秒（`api.DefaultAuthFloor`，
  用例里可注入更短的下限），快慢里读不出 token 对不对；请求级的 Bearer 校验不叠这个下限，只有一次 Argon2id。
- **界面是公开的**：`index.html` / `auth.html` / 静态资源不含数据，谁都拿得到；数据全在 `/api/*` 后面。
  浏览器进站先看 `localStorage` 里有没有 token，没有就去 `/auth`；任何 API 回 `401` 就清掉本地 token 并回认证页。
- token 就存在 `localStorage` 里：页面用 Vue 的插值渲染消息，没有任何用户内容进 `innerHTML`，
  不存在把 token 读走的注入点。
- 消息的正文与附件对所有进站的人可见、可改、可删；盘上是明文，没有第二种可见性。
- 应用不做 TLS / 限流，交给反代；应用只做输入上限与校验（体积、字段长度、类型）。
- 附件名只取最后一段并去掉控制字符，不当路径用。
- 数据目录 `0700`、消息文件与附件 `0600`，启动时收紧，`/readyz` 持续断言。
- `/livez` 永远 `200` 且不记日志；`/readyz` 口令不对回 `401`（带 `WWW-Authenticate: Bearer`）、
  依赖没就绪回 `503`，失败细节只进后端日志。

## 6. i18n

- 键是点号分层的符号名，中英各一张表，前后端各一份：后端 `i18n/`，前端 `frontend/src/i18n/`。
- 后端只负责「键 + 参数」的渲染（`Accept-Language` 决定语言），业务层不拼文案；
  前端负责界面文案与 `{name}` 占位符。
- 未知键原样返回键名，没有回退：漏配在界面上一眼可见。
