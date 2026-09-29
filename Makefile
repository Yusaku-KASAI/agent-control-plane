.PHONY: check lint fmt test build

check: lint test

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
