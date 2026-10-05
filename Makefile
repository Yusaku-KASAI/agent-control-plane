.PHONY: check lint fmt test build

check: lint test

SH_FILES := $(wildcard .devcontainer/*.sh) $(wildcard platform/plugins/*/bin/*)

lint:
	golangci-lint run ./...
	shellcheck $(SH_FILES)

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
