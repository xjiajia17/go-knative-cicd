# 运行镜像 = scratch + 一个静态 Go 二进制，最终约 10MB，里面没有 shell、没有包管理器、没有 CA 证书。
#
# 为什么不用常见的多阶段构建（FROM golang AS build）？
#   1. Go 交叉编译是原生能力：在 x86 的 GitHub runner 上 `GOARCH=arm64 go build` 直接产出
#      arm64 二进制，不需要 qemu 模拟，也不需要拉 300MB+ 的 golang 构建镜像。
#   2. 国内网络拉 docker.io/library/golang 很慢，而 scratch 不需要拉任何基础镜像。
#   3. 构建产物由 Makefile / CI 统一负责，镜像层只做"打包"，职责更清晰。
# 代价：Dockerfile 不自包含，必须先有 bin/linux/<arch>/server。`make image` 会把这两步串起来。
#
# 如果以后需要在容器里排查问题，用 `kubectl debug` 挂一个临时容器，而不是往镜像里塞 shell。
FROM scratch

# buildx 会自动注入 TARGETARCH；本地普通 docker build 时用默认值 arm64（本机 minikube 节点架构）。
ARG TARGETARCH=arm64

COPY bin/linux/${TARGETARCH}/server /server

# 非 root 运行。scratch 里没有 /etc/passwd，所以必须用数字 UID/GID。
USER 65532:65532

# Knative 会注入 PORT 环境变量（默认 8080），这里只是声明默认值。
ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/server"]