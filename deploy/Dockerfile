# 多阶段构建：Node 编译前端 → Go 编译单二进制 → 精简运行镜像
FROM node:22-alpine AS frontend
WORKDIR /fe
COPY web/frontend/package.json web/frontend/package-lock.json* ./
RUN npm ci --no-fund --no-audit
COPY web/frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /fe/../dist /src/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/qjrxy ./cmd/qingjin

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata &&     adduser -D -H -u 10001 qjrxy &&     mkdir -p /data && chown qjrxy:qjrxy /data
ENV TZ=Asia/Shanghai     QINGJIN_ADDR=:8090     QINGJIN_DATA_DIR=/data
COPY --from=builder /out/qjrxy /usr/local/bin/qjrxy
USER qjrxy
EXPOSE 8090
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/qjrxy"]
