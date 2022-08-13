all: build

build:
	@go build -o kklog main.go

run:
	@go run main.go 123456 -alias aaaaa

clean:
	@rm ./kklog

install:
	@echo "将 kklog 复制到 /usr/local/bin 下"
	@sudo cp ./kklog /usr/local/bin

.PHONY: all build run clean install
