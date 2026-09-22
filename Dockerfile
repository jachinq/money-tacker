# 前端：pnpm 走 npmmirror；产物仅为 web/dist
FROM node:22-alpine3.24 AS web
WORKDIR /web

ARG NPM_REGISTRY=https://registry.npmmirror.com
ENV NPM_CONFIG_REGISTRY=${NPM_REGISTRY} \
    COREPACK_NPM_REGISTRY=${NPM_REGISTRY}

COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

# 后端：Go module 走 goproxy.cn；静态链接、去掉符号表
FROM golang:1.26.5-alpine3.24 AS build
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=0 \
    GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY --from=web /web/dist ./web/dist

RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# 仅取出 CA、时区、非 root 用户，再装进 scratch
FROM alpine:3.24 AS os
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 65532 app \
    && mkdir -p /app/data /tmp \
    && chmod 1777 /tmp \
    && chown -R 65532:65532 /app

FROM scratch
COPY --from=os /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=os /usr/share/zoneinfo/Asia/Shanghai /usr/share/zoneinfo/Asia/Shanghai
COPY --from=os /etc/passwd /etc/passwd
COPY --from=os /etc/group /etc/group
COPY --from=os --chown=65532:65532 /tmp /tmp
COPY --from=os --chown=65532:65532 /app /app
COPY --from=build --chown=65532:65532 /out/server /app/server
COPY --from=web --chown=65532:65532 /web/dist /app/web/dist

WORKDIR /app
USER 65532:65532
ENV APP_ENV=prod \
    APP_ADDR=:8080 \
    APP_TZ=Asia/Shanghai \
    TZ=Asia/Shanghai \
    SQLITE_PATH=data/app.db

EXPOSE 8080
ENTRYPOINT ["/app/server"]
