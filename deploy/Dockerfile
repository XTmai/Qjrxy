# 多阶段构建，产物为不含编译器的精简镜像
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/qingjin ./cmd/qingjin

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -H -u 10001 qingjin && \
    mkdir -p /data && chown qingjin:qingjin /data
ENV TZ=Asia/Shanghai \
    QINGJIN_ADDR=:8090 \
    QINGJIN_DATA_DIR=/data
COPY --from=builder /out/qingjin /usr/local/bin/qingjin
USER qingjin
EXPOSE 8090
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/qingjin"]
