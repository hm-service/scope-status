# ==========================================
# 第一阶段：构建前端
# ==========================================
FROM --platform=$BUILDPLATFORM node:26-alpine AS frontend

# 与 frontend/package.json 的 packageManager 保持一致（这条 Node 线里没有 corepack）
ARG PNPM_VERSION=12.4.1
RUN npm install -g pnpm@${PNPM_VERSION}

WORKDIR /build/frontend

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend/ ./
RUN pnpm build

# ==========================================
# 第二阶段：构建 Go 二进制
# ==========================================
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

# CGO_ENABLED=0：纯静态链接，运行时不需要 libc
ENV CGO_ENABLED=0

# Go 模块代理：默认走国内镜像（海外构建可用 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /build

COPY go.* ./
RUN go mod download

COPY . .
COPY --from=frontend /build/frontend/dist ./frontend/dist

# -ldflags="-w -s"：移除调试信息与符号表，明显减小二进制
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -buildvcs=false -ldflags="-w -s" -trimpath -o app.bin .

# ==========================================
# 第三阶段：运行
# ==========================================
FROM alpine:3

# podman 把 kube 的 httpGet 探针翻译成 `curl -f <url> || exit 1` 在容器里执行，
# 镜像里没有 curl 探针就恒失败，容器会被反复判 unhealthy 后重启。
RUN apk --no-cache add ca-certificates curl tzdata

WORKDIR /app

# 运行时只有这一个二进制：前端产物已经 go:embed 进去，frontend/ 整个不进最终镜像
COPY --from=builder /build/app.bin .

ENV TZ=Asia/Shanghai \
    DATA_DIR=/data \
    HOST=0.0.0.0 \
    PORT=8080

VOLUME ["/data"]
EXPOSE 8080

CMD ["./app.bin"]
