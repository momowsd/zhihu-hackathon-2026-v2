FROM docker.m.daocloud.io/library/golang:1.24-alpine AS build

WORKDIR /src
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories
COPY go.mod go.sum* ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOMAXPROCS=1 GOMEMLIMIT=512MiB \
    go build -p=1 -o /out/server ./cmd/server

FROM docker.m.daocloud.io/library/alpine:3.21
WORKDIR /app
COPY --from=build /out/server /app/server
EXPOSE 8080
CMD ["/app/server"]
