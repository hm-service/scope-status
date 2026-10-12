set shell := ["bash", "-uc"]

# 让 recipe 拿到位置参数：提交信息是用户文本，走 {{ }} 插值会被 shell 再解释一遍
# （带引号、$ 或反引号的消息会被改样），所以 push 用 "$1"。
set positional-arguments

DIST := ".dist"
DIST_OS := env("DIST_OS", "linux")
ARCHS := env("ARCHS", "amd64 arm64")

# 列出全部命令
default:
    @just --list

# 安装依赖并构建前端到 frontend/dist
frontend:
    cd frontend && pnpm install --frozen-lockfile && pnpm build

# 造占位 frontend/dist/STUB，让 go:embed 能编译；占位版跑起来 / 是 404、/readyz 是 503
frontend-stub:
    @mkdir -p frontend/dist
    @test -f frontend/dist/index.html || touch frontend/dist/STUB

# 构建前端，并交叉编译全部架构到 .dist/app-<os>-<arch>（纯静态单文件）
build: frontend
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p {{ DIST }}
    for arch in {{ ARCHS }}; do
        printf '  %-20s' "{{ DIST_OS }}/$arch"
        CGO_ENABLED=0 GOOS={{ DIST_OS }} GOARCH="$arch" go build -o "{{ DIST }}/app-{{ DIST_OS }}-$arch" .
        echo ok
    done
    ls -l {{ DIST }}

# 本机跑起来（HOST / PORT / READYZ_TOKEN / TZ 等直接走环境变量）
run: frontend
    DATA_DIR={{ justfile_directory() }}/.data go run .

# 提交前跑一遍：Go 的 vet + test，加前端的类型检查（缺前端依赖会按锁文件装一次）
check: vet test
    cd frontend && pnpm install --frozen-lockfile
    cd frontend && pnpm typecheck

# 跑全部测试（没有前端产物就先造占位，Go 的测试不需要真前端）
test: frontend-stub
    go test ./... -count=1

# go vet
vet: frontend-stub
    go vet ./...

# go fmt 本模块的包
fmt:
    go fmt ./...

# 清掉构建产物与万一落在仓库里的 Go 缓存
clean:
    rm -rf {{ DIST }} .go-build
    chmod -R u+w .go-mod-cache 2>/dev/null || true
    rm -rf .go-mod-cache

# 提交并推送：just push "提交信息"（没有改动就只打印一行，不提交也不推）
push message:
    #!/usr/bin/env bash
    set -euo pipefail
    git add -A
    if git diff --cached --quiet; then
        echo "没有改动，跳过提交与推送"
        exit 0
    fi
    git commit -m "$1"
    git push
