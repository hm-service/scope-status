# AGENTS.md

本项目的骨架、目录与实现约定来自 `create-lite-go-web-app` 技能：`references/directory-conventions.md`
（目录）与 `references/implementation-conventions.md`（实现）。两份文档是规范的正文，这里只记本项目自己的东西；
冲突时以技能文档为准，别在这里复述一遍。

## 这个项目是什么

内网条目 / 服务 / 网站的连通状态看板，外加一个可传附件的消息面板。交付物是一个静态二进制，
前端产物 `go:embed` 进去；运行时状态全在数据目录：`<DATA_DIR>/source.yaml`（人写的配置）
与 `<DATA_DIR>/messages/`（消息 + 内容寻址的附件池）。

## 本项目的取舍（与规范的偏差及理由）

- 依赖只有 `gopkg.in/yaml.v3`：配置是人手写的 YAML，标准库没有解析器，其余一律标准库。
- 没有 `db/`：消息按「一消息一 json + `files/<sha256>` 附件池」落盘，不引入 SQLite。
- 口令消息的 `key` 在盘上是明文，门禁在服务端（没验过口令的响应一个字都不给）。这是 `DESIGN.md`
  里定下的取舍，要改成哈希先找用户确认。
- 探测要打到宿主 / WireGuard 网段，所以部署时网络路径（宿主机网络、反代可信源）比隔离更重要。

## 常用命令

`just --list` 看全部；日常是 `just check`（vet + test + 前端 typecheck）、`just build`（交叉编译到 `.dist/`）、
`just run` / `just stop`（本机起停，`DATA_DIR` / `PORT` 可用环境变量覆盖）、`just push "提交信息"`。

## 不要做的事

- `DESIGN.md` 由用户主导：不擅自改，发现问题只给建议 + 理由。
- 不引入框架、断言库、eslint / 测试框架这类额外工具链。
- 不写兜底分支：数据或配置不合法就在启动时明确报错。
