FROM docker.m.daocloud.io/library/golang:1.24-alpine AS build

WORKDIR /src
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories \
    && apk add --no-cache gcc musl-dev
COPY go.mod go.sum* ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
COPY . .
# mattn/go-sqlite3 (via gorm.io/driver/sqlite) requires CGO; CGO_ENABLED=0 breaks the build.
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOMAXPROCS=1 GOMEMLIMIT=700MiB \
    CGO_CFLAGS="-D_LARGEFILE64_SOURCE" \
    go build -p=1 -o /out/server ./cmd/server

FROM docker.m.daocloud.io/library/alpine:3.21
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8080
CMD ["/app/server"]
