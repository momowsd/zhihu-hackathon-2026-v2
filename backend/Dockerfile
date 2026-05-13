# 构建上下文须为仓库根目录（见 docker-compose.yml），以便打入 eval-workspace 下领域与模型互评冷启数据。
FROM docker.m.daocloud.io/library/golang:1.24-alpine AS build

WORKDIR /src
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories \
    && apk add --no-cache gcc musl-dev
COPY backend/go.mod backend/go.sum* ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
COPY backend/ .
# mattn/go-sqlite3 (via gorm.io/driver/sqlite) requires CGO; CGO_ENABLED=0 breaks the build.
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOMAXPROCS=1 GOMEMLIMIT=700MiB \
    CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
    go build -p=1 -o /out/server ./cmd/server

FROM docker.m.daocloud.io/library/alpine:3.21
WORKDIR /app
COPY --from=build /out/server /app/server
COPY eval-workspace/domains /app/eval-workspace/domains
COPY eval-workspace/model-peer-evals /app/eval-workspace/model-peer-evals
ENV EVAL_WORKSPACE_DOMAINS=/app/eval-workspace/domains
ENV EVAL_MODEL_PEER_EVALS=/app/eval-workspace/model-peer-evals
EXPOSE 8080
CMD ["/app/server"]
