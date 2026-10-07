.PHONY: check lint fmt test build

check: lint test

SH_FILES := $(wildcard .devcontainer/*.sh) $(wildcard platform/plugins/*/bin/*)

lint:
	golangci-lint run ./...
	shellcheck $(SH_FILES)
	go mod tidy -diff
	govulncheck ./...
	claude plugin validate ./platform
	python3 .devcontainer/check-vscode-settings.py

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
