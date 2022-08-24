all: build

build:
	@go build -o ./build/kklog *.go

run:
	@go run main.go config_util.go

clean:
	@rm ./kklog

install:
	@echo "将 kklog 复制到 /usr/local/bin 下"
	@sudo mv ./build/kklog /usr/local/bin

.PHONY: all build run clean install
