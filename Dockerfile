# 构建阶段一：前端。产物与目标架构无关，任何架构的宿主机都产出同样的 dist。
FROM node:22-alpine AS web
WORKDIR /src/web
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 构建阶段二：Go 静态二进制。
# TARGETARCH 由 buildx 注入（linux/amd64 → amd64，linux/arm64 → arm64），用于交叉编译；
# 直接用 docker build（legacy builder 或原生）时该变量为空，回退到本机架构，
# 所以在 arm64 机器上 `docker build` 就是原生产物，在 x86 上用 buildx 也不必跑 QEMU 编 Go。
FROM golang:1.23-alpine AS build
ARG TARGETARCH
# ACL4SSR 规则数据的 revision，由 CI 查询最新提交后注入（见 .github/workflows/image.yml）。
# 不传时用代码里的兜底 revision，构建照常成功。
ARG ACL4SSR_REV=""
WORKDIR /src
ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN set -eu; \
    arch="${TARGETARCH:-}"; \
    [ -n "$arch" ] || arch="$(go env GOHOSTARCH)"; \
    ldflags="-s -w"; \
    if [ -n "$ACL4SSR_REV" ]; then \
        ldflags="$ldflags -X towerdocker/internal/scheme.ACL4SSRRevision=$ACL4SSR_REV"; \
    fi; \
    echo "building linux/$arch, acl4ssr=${ACL4SSR_REV:-default}"; \
    GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags="$ldflags" -o /out/hangbiao .

# 运行阶段
# 国内网络拉不到 dl-cdn.alpinelinux.org 时，加 --build-arg APK_REPO=https://mirrors.tuna.tsinghua.edu.cn/alpine
FROM alpine:3.20
ARG APK_REPO=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPO}|g" /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/hangbiao /app/hangbiao
VOLUME ["/data"]
EXPOSE 8630
ENTRYPOINT ["/app/hangbiao"]
CMD ["-addr", ":8630", "-data", "/data"]
