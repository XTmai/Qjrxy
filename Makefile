.PHONY: build run test vet clean linux windows mac docker

BINARY := bin/qjrxy

build:
	go build -trimpath -o $(BINARY) ./cmd/qingjin

run: build
	./$(BINARY) -addr :8080 -data ./data

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
