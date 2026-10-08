.PHONY: check check-go check-platform check-target fmt test build

# One target per concern, so that CI can run each from only the files it reads (docs/target/README.md).
check: check-go check-platform check-target

check-go:
	golangci-lint run ./...
	go mod tidy -diff
	govulncheck ./...
	go test -race ./...

check-platform:
	claude plugin validate ./platform
	shellcheck platform/plugins/*/bin/*

check-target:
	shellcheck .devcontainer/*.sh
	python3 .devcontainer/check-vscode-settings.py

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
