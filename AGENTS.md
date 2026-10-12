# AGENTS.md

给基于本模板开工的 Agent 看的基本约定。项目自己的页面、接口、表结构写进 `DESIGN.md`，这里不重复。

参考项目：<http://github.com/hm-service/scoped-skin/>

- 显式大于隐式
- 约定大于配置
- 不变量大于变量
- 安全大于效率
- 可理解大于性能
- 错误应该被尽早发现（fast fail）
- 宁可接受初期的反复迭代也不引入过多的兜底代码
- 除非真的有必要否则不应该引入依赖
- 只要条件允许，总是采用最新的正式版工具链
- 局部量大于全局量
- 高内聚优先于低耦合

## 1. 技术栈

- 交付物期望是一个静态二进制文件。
- 应当总是采用最新的正式版工具链。确保项目 / CI / Dockerfile 对齐。
- 本项目优先被设计为容器环境运行，面向 Podman。
- 如无必要和明确同意，请勿引入其他工具链。

### 1.1. 后端

- 尽可能保持纯 Go，无 CGO。
- 后端采用 Go，HTTP 处理只用标准库 `net/http`。
- 存储若需要使用 SQLite，采用 `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`）。
- 后端负责托管前端静态文件：`go:embed` 把 `frontend/dist` 编入，嵌入声明写在仓库根的 `web.go`。
- 静态文件托管并发量默认通过信号量限制为 `MAX_STATIC_CONCURRENCY`，API 路由（包括 /readyz 和 /livez）默认通过信号量限制为 `MAX_API_CONCURRENCY`，两者使用独立槽位。

**创建模板项目**:

```bash
go mod init <app-name>
```

### 1.2. 前端

- 前端工程全量约束在 `./frontend` 目录中，构建结果指定为 `./frontend/dist` 目录。
- 前端采用 pnpm 作为包管理器，Vue + Vite + TypeScript 作为工具链。

**创建模板项目**:

```bash
pnpm create vite frontend --template vue-ts --no-interactive --no-immediate
cd frontend && pnpm install
```

开发运行: `cd frontend && pnpm dev`

构建: `cd frontend && pnpm build`

检验: `cd frontend && pnpm typecheck`（vue-tsc，脚手架自带，`pnpm build` 也会先跑它）。暂时只用脚手架自带的工具，不引入 eslint / 测试框架等额外包。

### 1.3. 命令入口

采用 `just`：

- `just build` 先构建前端、再交叉编译全部架构到 `.dist/`。
- 其他命令可用 `just` 命令直接查看。

## 2. 目录说明

### 2.1. 源码与交付物（示例）

| 路径                           | 说明                                                                        |
| ------------------------------ | --------------------------------------------------------------------------- |
| `main.go`                      | 入口：读环境变量 → 建应用容器 → 挂路由 → 监听（只做接线）                   |
| `web.go`                       | 根目录：`go:embed frontend/dist`，把前端产物编进二进制（`Dist`）            |
| `justfile`                     | 命令入口：`just --list` 看全部命令                                          |
| `api/`                         | HTTP 层：路由、中间件、语言协商、探针、页面与站点接口                       |
| `app/`                         | 用例层：权限判定，及启动时的一次性工作（建表 / 收紧权限 / 建初始账号）      |
| `core/`                        | 纯逻辑：哈希、token、会话、签名、UUID、校验、审计、文件权限；不 import 上层 |
| `db/`                          | SQLite：打开、建表、查询、事务；SQL 只出现在这里                            |
| `httpx/`                       | HTTP 小工具：JSON 响应、客户端 IP、错误 → 状态码映射                        |
| `i18n/`                        | 后端文案表与 T(lang, key, args…)                                            |
| `frontend/`                    | 前端工程（pnpm + Vite + Vue + TS），构建产物 `frontend/dist`                |
| `builtin/`                     | 随二进制发布的内置资源（go:embed + manifest）                               |
| `internal/testenv/`            | 测试脚手架：临时 DATA_DIR、临时库、现成的测试服务                           |
| `tools/`                       | 零依赖小工具（如 `shot.mjs`：CDP 截图 + 控制台错误转发）                    |
| `docs/`                        | 交付物：截图、反代示例；README 可直接引用它们                               |
| `k8s.yaml` `quadlet-kube.kube` | 部署：Pod + PVC + ConfigMap；podman kube play / systemd 模板                |
| `DESIGN.md`                    | 有用户主导编写的设计：包括部署、页面、接口、表结构、安全模型等方面          |

- 除非 `DESIGN.md` 为空或用户明确要求修改，否则不应自行改动，仅做汇报与建议。

### 2.2. 点号目录（示例）

点号开头的目录多为本地状态与产物，只有 `.github/` 是例外（属于源码、入库）：

| 目录                                     | 入库 | 说明                                                                   |
| ---------------------------------------- | ---- | ---------------------------------------------------------------------- |
| `.github/`                               | 是   | 唯一例外：CI 工作流，属于源码                                          |
| `.data/`                                 | 否   | 接近生产环境的运行时数据：数据库（含 `-wal`/`-shm`）、密钥等；允许重建 |
| `.scratch/`                              | 否   | 概念验证目录，当需要用一个最小项目验证某个概念时，将代码限定于此       |
| `.data_sample/`                          | 否   | 示例素材，通常用于手工铺一份演示数据用                                 |
| `.dist/`                                 | 否   | 构建产物（`just build`：`app-<os>-<arch>`）                            |
| `.shots/`                                | 否   | 验证截图与本机临时脚本；要进 README 的挪到 `docs/`                     |
| `.go-build/` `.go-mod-cache/`            | 否   | 默认 Go 缓存不可写时的临时重定向                                       |
| `.ref/`                                  | 否   | 本地 clone 的参考项目，只查阅、不修改                                  |
| `.prototype/`                            | 否   | 换实现语言时归档的旧版本，只作参考、不再维护                           |
| `.agents/` `.codex/` `.claude/` `.venv/` | 否   | agent / 编辑器 / 语言运行时自带的目录，按需忽略                        |

约定：本地与临时目录一律 `.` 前缀（`.github/` 是唯一例外）。用不到的目录不要留着占位；`*_test.go` 与源码同目录，不单列测试目录。

## 3. 基本约定

### 3.1. 环境变量

通常应有以下通用环境变量

| 变量名                   | 示例（默认值）       | 说明                                         |
| ------------------------ | -------------------- | -------------------------------------------- |
| `DATA_DIR`               | /data                | 数据存储目录（如果有数据存储）               |
| `TZ`                     | Asia/Shanghai        | 时区信息                                     |
| `HOST`                   | 127.0.0.1            | 服务监听主机                                 |
| `PORT`                   | 8080                 | 服务监听端口                                 |
| `READYZ_TOKEN`           | your_readyz_token    | /readyz?token 端点 token                     |
| `TRUSTED_PROXIES`        | 127.0.0.1/32,::1/128 | 反代可信源                                   |
| `MAX_API_CONCURRENCY`    | 128                  | API 路由（包括 /readyz 与 /livez）最大并发量 |
| `MAX_STATIC_CONCURRENCY` | 64                   | 静态文件服务最大并发量                       |
| `GOMEMLIMIT`             | 128MiB               | Go GC 阈值                                   |

### 3.2. 注释

- 非必要不注释。注释只写「为什么」，不写「这里做了什么」——代码本身能说清的都不写。
- 注释必须与代码同步，过时注释按缺陷处理。
- 如无特殊必要，注释中不要过于具体引用文档，因为文档可能变动，过于具体的引用极易产生引用漂移。
- 默认使用中文写注释。

### 3.3. 分层与依赖方向

**后端**:

- `api → app → core / db`，单向。`core` 不许 import 上层；需要环境就往函数里传参数。
- 不建 util / common 大杂烩包；两个包都要用的东西就下沉到 `core`。

### 3.4. i18n

- 如无特殊说明，默认支持简体中文和英文两种语言。
- 标识符与服务日志统一用英文，面向用户的文案无论前后端一律走 i18n，不硬编码。
- 键是点号分层的符号名（`library.search-placeholder`），每条同时给中英两栏；前后端同一套策略，各有一张表。
- 未知键原样返回键名，不采用回退机制，漏配应能在界面上一眼看见。
- 后端占位符 `%d` / `%s`，前端 `{name}`；业务层只抛「键 + 参数」，渲染成哪种语言由 HTTP 层按 `Accept-Language` 决定。

### 3.5. 测试

- 只用标准库 `testing`，不引第三方断言库；表格驱动 + `t.Run` 子用例。
- 需要数据目录就用 `internal/testenv` 造临时实例，测试不得碰真实 `DATA_DIR`，如果确实需要真实库，可以将真实库复制一份专用。
- 每修一个 bug 补一条回归用例（`regress_test.go`），钉住的是「之前错在哪」。
- `go test ./... -count=1` 不吃缓存；覆盖率按包记进 README，写实测值。
- 受时间约束的逻辑（限流、恒定时间比较）要能注入或压缩，别让用例真的睡 1 秒。

### 3.6. 前端验证

- 先检查本机是否有 `chrome` 和配套的 `chromedriver` 可用。
- 改 UI 后应实跑浏览器：`tools/shot.mjs` 截图 + 转发控制台错误。
- 无头浏览器默认 `en-US`，要验中文得显式设 `localStorage`；关键截图进 `docs/` 并在 README 里引用。
- 给前端留一个调试入口（如 `window.__app = { store, viewer }`），排查时用 CDP 读状态比翻日志快。

### 3.7. 数据与兼容

- 除非用户明确要求，否则默认改项目目录下的所有数据都可直接删库重建，无需考虑任何向后兼容性。
- 默认不保留旧库数据：结构变了就直接改，数据可重建。
- 不为假设里的旧数据写兜底分支；宁可启动时明确报错，减少冗余代码，也不静默降级。
- 表结构与字段约束写进 `DESIGN.md`，与 `db/` 的建表语句保持一致。

### 3.8. 日志与安全

- 不落日志文件：结构化 JSON 走 stdout，由容器运行时收集；日志统一采用英文（面向运维），如果涉及到面向用户的文案，只记录 i18n 键。
- 业务校验错误在日志里给「文案键 + 参数」，渲染成哪种语言只发生在 HTTP 响应里。
- 一条请求一行访问日志，每行记录至少包括真实客户端 IP 与时间戳（使用 RFC 3339 带时区，采用 TZ 时区）。
- 密码哈希 + 恒定时间比较；私钥与数据库 `0600`、数据目录 `0700`，启动时收紧、/readyz 就绪探针持续断言。
- 应用不考虑 TLS、限流等网关工作，交给反代完成，只做输入上限与校验：请求体大小、字段长度、类型。

### 3.9. Git 提交

- 提交前，根据修改情况复查涉及到的代码，清里本次改动波及的过时注释与死代码。
- 提交消息保持语言精炼，只概要说明关键变化。
- 提交前先确认 `just check` 全绿（Go 的 vet/test + 前端的 typecheck）、工作树干净，`.gitignore` 正确忽略项目无关文件。

### 3.10. 部署与运维

- `k8s.yaml` 供 podman `kube play` 与 systemd 模板用：ConfigMap 注入环境变量，`DATA_DIR` 挂 PVC；容器开 `readOnlyRootFilesystem` 并 `drop: ALL`。探针分两个：`/livez` 只看进程（不记日志），`/readyz` 查依赖就绪并记一条审计日志（可用 `READYZ_TOKEN` 挡住外部探测）。
- 镜像同时推「日期」与 `latest` 两个 tag，`linux/amd64` + `linux/arm64`；Go 在 `$BUILDPLATFORM` 上交叉编译。
- 前端也在镜像里构建（node 阶段 → `COPY --from` 把 `frontend/dist` 交给 Go 阶段），所以 `.dockerignore` 排掉宿主机的 `frontend/node_modules` 与 `frontend/dist`；运行时阶段只放那一个二进制。
- 数据目录（SQLite 连同 `-wal` / `-shm`）应就是应用全部状态。
