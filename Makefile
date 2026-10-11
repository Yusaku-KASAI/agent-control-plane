.PHONY: check check-go check-platform check-target check-docs fmt test build

# One target per concern, so that CI can run each from only the files it reads (docs/target/contract.md).
check: check-go check-platform check-target check-docs

check-go:
	golangci-lint run ./...
	go mod tidy -diff
	govulncheck ./...
	go test -race ./...

check-platform:
	claude plugin validate ./platform
	shellcheck $(wildcard platform/plugins/*/bin/* platform/plugins/*/scripts/*)

check-target:
	shellcheck .devcontainer/*.sh
	python3 .devcontainer/check-vscode-settings.py

check-docs:
	python3 tools/check-docs.py

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
