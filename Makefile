.PHONY: build run test vet clean linux windows mac docker release frontend frontend-dev

BINARY := bin/qjrxy
VERSION := 1.6.4
RELEASE := release

# 前端产物（web/dist）是 go:embed 的输入。只改后端时不需要跑这个。
frontend:
	npm --prefix web/frontend install
	npm --prefix web/frontend run build

# 前端热更新，/api 代理到 :8090（需先另开一个终端跑 make run）
frontend-dev:
	npm --prefix web/frontend run dev

build:
	go build -trimpath -o $(BINARY) ./cmd/qingjin

run: build
	./$(BINARY) -addr :8090 -data ./data

vet:
	go vet ./...

test:
	go test ./...

clean:
	rm -rf bin

# 交叉编译：Linux amd64（服务器部署）
linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/qjrxy-linux-amd64 ./cmd/qingjin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/qjrxy-linux-arm64 ./cmd/qingjin

windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/qjrxy.exe ./cmd/qingjin

mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/qjrxy-darwin-arm64 ./cmd/qingjin

docker:
	docker build -f deploy/Dockerfile -t qjrxy .

# 发布包：四平台二进制 + sha256 校验和 + 汇总 zip（输出到 release/）
# 注意 web/dist 是 go:embed 的输入，打包前先 npm --prefix web/frontend run build
release:
	rm -rf $(RELEASE) && mkdir -p $(RELEASE)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $(RELEASE)/qjrxy-linux-amd64 ./cmd/qingjin
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $(RELEASE)/qjrxy-linux-arm64 ./cmd/qingjin
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o $(RELEASE)/qjrxy-windows-amd64.exe ./cmd/qingjin
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o $(RELEASE)/qjrxy-darwin-arm64 ./cmd/qingjin
	cd $(RELEASE) && ( command -v sha256sum >/dev/null 2>&1 && sha256sum qjrxy-* > sha256sums.txt || shasum -a 256 qjrxy-* > sha256sums.txt )
	cd $(RELEASE) && zip -q qjrxy-v$(VERSION)-all.zip qjrxy-linux-amd64 qjrxy-linux-arm64 qjrxy-windows-amd64.exe qjrxy-darwin-arm64 sha256sums.txt
	@echo "release/qjrxy-v$(VERSION)-all.zip 已生成"
