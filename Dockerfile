# 目标 bifrost 发布版本及其内嵌的 core 版本。
# 升级 bifrost 时保持三者同步: BIFROST_TAG / BIFROST_VERSION / CORE_VERSION。
ARG BIFROST_TAG=transports/v1.6.9
ARG BIFROST_VERSION=v1.6.9
ARG CORE_VERSION=v1.7.7
ARG PLUGIN_DIR=plugins

# ---- 拉取 bifrost 源码 ----
FROM alpine:3.23.4 AS bifrost-source
ARG BIFROST_TAG
RUN apk add --no-cache git && \
    git clone --depth 1 --branch "${BIFROST_TAG}" https://github.com/maximhq/bifrost.git /src

# ---- UI 构建 ----
FROM node:25-alpine3.23@sha256:bdf2cca6fe3dabd014ea60163eca3f0f7015fbd5c7ee1b0e9ccb4ced6eb02ef4 AS ui-builder
WORKDIR /app
RUN apk upgrade --no-cache
COPY --from=bifrost-source /src/ui/package*.json ./
RUN npm ci
COPY --from=bifrost-source /src/ui ./
RUN npm run build-enterprise

# ---- Go Build Stage: Compile the Go binary ----
FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS builder
WORKDIR /app
RUN apk upgrade --no-cache && \
    apk add --no-cache gcc musl-dev sqlite-dev binutils binutils-gold
ENV CGO_ENABLED=1 GOOS=linux
COPY --from=bifrost-source /src/transports/go.mod /src/transports/go.sum ./
RUN ls
RUN cat go.mod
RUN go mod download
COPY --from=bifrost-source /src/transports/ ./
COPY --from=ui-builder /app/out ./bifrost-http/ui
ENV GOWORK=off
ARG VERSION=unknown
RUN go build \
-ldflags="-w -s -X main.Version=v${VERSION}" \
-trimpath \
-o /app/main \
./bifrost-http
RUN test -f /app/main || (echo "Build failed" && exit 1)

# ---- 构建插件 ----
# TODO

# ---- 运行时 ----
FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
WORKDIR /app
RUN apk upgrade --no-cache && \
apk add --no-cache musl libgcc ca-certificates zlib
COPY --from=builder /app/main .
COPY --from=builder /app/docker-entrypoint.sh .
ARG ARG_APP_PORT=8080
ARG ARG_APP_HOST=0.0.0.0
ARG ARG_LOG_LEVEL=info
ARG ARG_LOG_STYLE=json
ARG ARG_APP_DIR=/app/data
ENV APP_PORT=$ARG_APP_PORT \
APP_HOST=$ARG_APP_HOST \
LOG_LEVEL=$ARG_LOG_LEVEL \
LOG_STYLE=$ARG_LOG_STYLE \
APP_DIR=$ARG_APP_DIR
ENV GOGC="" \
GOMEMLIMIT=""
RUN mkdir -p "$APP_DIR/logs" && \
adduser -D -s /bin/sh appuser && \
chown -R appuser:appuser /app && \
chown -R appuser:0 "$APP_DIR" && \
{ [ "$APP_DIR" = "/app" ] || chmod -R g=rwX "$APP_DIR"; } && \
chmod +x /app/docker-entrypoint.sh
USER 1000:0
VOLUME ["${APP_DIR}"]
EXPOSE $APP_PORT
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
CMD wget -q -O /dev/null http://127.0.0.1:${APP_PORT}/health || exit 1
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/main"]
