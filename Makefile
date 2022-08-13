all: build

build:
	@go build -o kklog main.go

run:
	@go run main.go 123456 -alias aaaaa

.PHONY: clean

clean:
	@rm -rf ./kklog
