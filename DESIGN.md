# DESIGN.md

Scope Status 的设计：内网 WireGuard 连通状态看板。单文件静态二进制，容器里跑，数据目录即全部状态。

## 1. 部署

| 环境变量                 | 默认值                 | 说明                                                     |
| ------------------------ | ---------------------- | -------------------------------------------------------- |
| `DATA_DIR`               | `/data`                | 数据目录：配置 `source.yaml` 与消息 `messages/` 都在这里 |
| `TZ`                     | `Asia/Shanghai`        | 时区；影响日志时间戳与消息文件名                         |
| `HOST`                   | `127.0.0.1`            | 监听地址                                                 |
| `PORT`                   | `8080`                 | 监听端口                                                 |
| `READYZ_TOKEN`           | 空                     | 非空时 `/readyz` 必须带 `?token=`；空则不校验            |
| `TRUSTED_PROXIES`        | `127.0.0.1/32,::1/128` | 可信反代来源，决定真实客户端 IP 怎么取                   |
| `STREAM_INTERVAL`        | `5`                    | SSE 循环探测间隔秒数；`0` 表示不起循环（只按需探测）     |
| `MAX_API_CONCURRENCY`    | `128`                  | API 路由（含 `/livez` `/readyz`）并发上限                |
| `MAX_STATIC_CONCURRENCY` | `64`                   | 静态文件并发上限                                         |
| `GOMEMLIMIT`             | `128MiB`               | Go GC 阈值，由运行时自己读                               |
| `MAX_UPLOAD_BYTES`       | `1073741824`           | 单个附件上限（1 GiB）                                    |

- 配置与状态同目录，配置路径写死为 `<DATA_DIR>/source.yaml`：容器把配置当 ConfigMap 挂在数据目录里，
  状态挂 PVC；挂一个卷就是应用的全部。
- 启动即失败：配置文件不存在 / 解析不通过 / 数据目录建不出来，进程直接报错退出，不做任何降级。
- 配置路径没有第二个来源，也没有环境变量能改它；改 `DATA_DIR` 就等于改配置路径，界面上与 `/api/health` 里显示的永远是当前生效的那一份。

## 2. 页面

四个标签页，顶栏常驻：标题、SSE 状态、语言切换、主题切换、刷新、重载配置（窄屏收进 `⋯` 菜单）。

| 标签页 | 内容                                                               |
| ------ | ------------------------------------------------------------------ |
| 状态   | 内网条目：一行一个，地址、延迟、连通性；按 `category` 分组，可折叠 |
| 服务   | 游戏服 / 站点后端：按 `protocol` 探测；`connect_cmd` 可复制        |
| 网站   | 链接：站点图标 + 名字可点开；显示 HTTP 状态                        |
| 消息   | 文本与附件；普通消息所有人可见，口令消息需输入口令；按时间戳倒序   |

- 每页顶部一条概况栏（总数 / 在线 / 离线，消息页是总数 / 待解锁 / 最近消息），四个页高度一致，
  切换标签不抖。
- 主题：默认浅色，可切深色，记在 `localStorage`；≤560px 走卡片式布局。
- 语言：默认按 `navigator.language`（`zh*` → 简体中文，其余 → English），顶栏可切换，记在
  `localStorage`；每次 `fetch` 带 `Accept-Language`，服务端错误文案跟着一起切。

## 3. 接口

除附件下载外一律 `application/json`；错误是纯文本（按 `Accept-Language` 渲染好的文案），状态码见表。

| 方法         | 路径                                    | 说明                                                         |
| ------------ | --------------------------------------- | ------------------------------------------------------------ |
| `GET`        | `/api/status`                           | 当前共享快照；还没有快照时 `204`                             |
| `GET`        | `/api/status?force=1`                   | 强制探一次再回                                               |
| `GET`        | `/api/events`                           | SSE：先补一帧 `messages`，再推 `snapshot` / `messages`       |
| `POST` `GET` | `/api/reload`                           | 重读 `<DATA_DIR>/source.yaml`，成功则立刻探一次              |
| `GET`        | `/api/health`                           | 运行状态与计数                                               |
| `GET`        | `/api/messages`                         | 未删除消息列表（口令消息未解锁只给 `id` / `createdAt`）      |
| `POST`       | `/api/messages`                         | 新建；`201`                                                  |
| `GET`        | `/api/messages/{id}?key=`               | 看一条；口令消息要 `key`                                     |
| `PATCH`      | `/api/messages/{id}`                    | 改正文 / 附件；口令消息要 `key`                              |
| `POST`       | `/api/messages/{id}/unlock`             | 用请求体里的 `key` 解锁一条                                  |
| `POST`       | `/api/messages/{id}/delete`             | 打删除标志位并回收无人引用的附件                             |
| `PUT`        | `/api/messages/files?name=<文件名>`     | 附件上传：请求体就是裸文件流，不拼 multipart                 |
| `GET`        | `/api/messages/files/{sha256}?id=&key=` | 下载附件；附件池共享，必须指明是哪个消息引用的它             |
| `GET`        | `/livez`                                | 进程活着就 `200`，不记日志                                   |
| `GET`        | `/readyz?token=`                        | 依赖就绪（配置已载入、数据目录可写）才 `200`，记一条审计日志 |
| `GET`        | `/` 及其余路径                          | 内嵌前端产物；未知 `/api/*` 回 `404` JSON                    |

`POST /api/reload` 成功时回 `{ok, message, friends, services, links, reloadCount, sourcePath, snapshot}`：
`message` 是服务端按 `Accept-Language` 渲染好的一句提示，`snapshot` 就是下面那份快照（`reused: false`）。
失败时回 `500` 纯文本，内容是配置校验的报错原文，旧配置继续生效。

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
      "id": "friend", "label": "朋友", "order": 0, "total": 2, "online": 1,
      "friends": [
        {
          "name": "小明", "note": null, "category": "friend", "online": true,
          "latencyMs": 12.4,
          "addresses": [
            { "address": "10.0.0.2", "status": "up", "latencyMs": 12.4, "detail": null, "method": "icmp" }
          ],
          "properties": { "面板账号": "example-pass" }
        }
      ]
    }
  ],
  "friends": [],
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

- `friends` 是扁平的全量列表，`groups` 是按 `category` 分好组的同一批对象；`services` /
  `servicesGroups` 同理。
- `status` 只有 `up` / `down`；判不出结论（没有 `ping`、域名解析不了）时整个探测报错，
  不编状态：`/api/status?force=1` 会回 `500` 纯文本，SSE 循环则记 traceback 后终止。

### 3.2. SSE

```
event: messages
data: {"messages":[…],"summary":{"total":1,"locked":0}}

event: snapshot
data: {…快照，多一个 "reused": false…}

: ping
```

- 连上先补一帧 `messages`（当前列表），再补一帧 `snapshot`（有快照时，`reused: true`）
  或 `hello`（`{"streamInterval":5}`）。
- 之后每轮探测完成推一帧 `snapshot`；消息有任何变更推一帧 `messages`。
- 15 秒没事件就发一个注释帧 `: ping` 保活；队列满时丢最旧的一帧（订阅者掉队不拖累服务端）。
- 长期没有订阅者时探测降到每分钟一次。

## 4. 存储

### 4.1. `<DATA_DIR>/source.yaml`

这份文件是本机手写的，里面通常有密码，所以整个数据目录都不入库；入库的只有 `source.example.yaml` 这份示例。
路径固定成 `DATA_DIR` 根下的 `source.yaml`，名字不再是可配置项。

顶层 mapping，四块：`friends` / `services` / `links` / `categories`（`probe`、`title` 可选）。
顶层直接写数组等价于 `friends`。字符串字段一律要求非空，类型不对就报错，不猜、不转换。

| 字段            | 适用     | 约束                                                                       |
| --------------- | -------- | -------------------------------------------------------------------------- |
| `name`          | 三者     | 非空字符串，同一页内不要求唯一                                             |
| `addresses`     | friends  | 非空数组，每项 IP 或域名（可带 `:端口`），去重                             |
| `category`      | 三者     | 字符串，转小写、空格转 `-`；缺省 `friend` / 服务是 `service`               |
| `properties`    | 三者     | 名称 → 值的映射，按书写顺序展示；值可以是数字、布尔                        |
| `note`          | friends  | 备注                                                                       |
| `host` `port`   | services | `host` 非空；`port` 1–65535                                                |
| `protocol`      | services | `udp` / `tcp` / `a2s`                                                      |
| `connect_cmd`   | services | 可复制的连接命令                                                           |
| `url`           | links    | `http(s)://` 开头的完整地址                                                |
| `probe_url`     | links    | 探测地址；不写就探 `url`                                                   |
| `expect_status` | links    | 视为「通」的 HTTP 状态码，默认 `200`                                       |
| `probe`         | 顶层     | `method`（`icmp`/`tcp`/`http`）、`timeout`、`port`、`concurrency`、`count` |
| `categories`    | 顶层     | `[{id, label, order}]`；没声明的分类用内置名字补齐                         |

### 4.2. `DATA_DIR/messages`

盘上只有两样东西，不分消息类型：

- `messages/message-yyyy-MM-dd-HH-mm-ss-ms(4)-随机6位.json` —— 一条消息一个文件。
  创建时间只在文件名里（唯一真相来源），消息本体按原样存：

  ```json
  {
    "kind": "public",
    "text": "正文",
    "updatedAt": 1760273703085,
    "attachments": [{ "sha256": "<64 hex>", "name": "豆包.png", "size": 3872014 }],
    "isDeleted": false
  }
  ```

  口令消息多一个 `key`。读到不合规的文件（缺字段、类型不对、id 对不上文件名）就是 `500`，
  不修复、不跳过。

- `messages/files/<sha256>` —— 附件池，按内容寻址，同内容天然一份。上传时边写边算 sha256，
  写完按内容改名；池子里已有就丢掉刚写的临时文件。

删除用 `isDeleted` 标志位：json 原地保留，随后扫一遍所有 `message*.json`，只回收不再被任何
**未删除**消息引用的池内文件。回收只由删除触发，孤儿附件（传了又没发）等下一次删除才会被扫掉。

## 5. 安全模型

- 口令消息的正文在盘上是**明文**，门禁完全在服务端：没验过 `key` 的响应里除了 `id` 和
  `createdAt` 什么都不给，附件也不暴露；下载附件同样要带 `key`。
- 登录态不存在：口令只存活在浏览器内存里，刷新即失效。
- 应用不做 TLS / 限流，交给反代；应用只做输入上限与校验（体积、字段长度、类型）。
- 附件名只取最后一段并去掉控制字符，不当路径用；口令用恒定时间比较。
- 数据目录 `0700`、消息文件 `0600`，启动时收紧，`/readyz` 持续断言。

## 6. i18n

- 键是点号分层的符号名，中英各一张表，前后端各一份：后端 `i18n/`，前端 `frontend/src/i18n/`。
- 后端只负责「键 + 参数」的渲染（`Accept-Language` 决定语言），业务层不拼文案；
  前端负责界面文案与 `{name}` 占位符。
- 未知键原样返回键名，没有回退：漏配在界面上一眼可见。
