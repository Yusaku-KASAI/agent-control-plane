.PHONY: check check-go check-platform check-target check-docs fmt test build

# One target per concern, so that CI can run each from only the files it reads.
check: check-go check-platform check-target check-docs

check-go:
	# govulncheck runs on .devcontainer/Dockerfile's Go, while acp is built with Dockerfile's; a bump
	# to only one of them would pass unnoticed.
	a=$$(sed -n 's/^ARG GO_VERSION=//p' Dockerfile); b=$$(sed -n 's/^ARG GO_VERSION=//p' .devcontainer/Dockerfile); \
		test -n "$$a" && test "$$a" = "$$b" \
		|| { echo "GO_VERSION missing or differs: Dockerfile=$$a, .devcontainer/Dockerfile=$$b"; exit 1; }
	golangci-lint run ./...
	go mod tidy -diff
	govulncheck ./...
	go test -race ./...

check-platform:
	claude plugin validate ./platform
	# Only the shell scripts: a command may be written in another language (conventions' check-docs).
	shellcheck $$(grep -lE '^#!.*[/ ](ba)?sh$$' $(wildcard platform/plugins/*/bin/* platform/plugins/*/scripts/* platform/plugins/*/tests/*))
	# Every plugin's tests, found by name, so that a new one runs without being listed here.
	failed=; for t in platform/plugins/*/tests/*-test.sh; do echo "$$t"; "$$t" || failed="$$failed $$t"; done; \
		test -z "$$failed" || { echo "failed:$$failed"; exit 1; }

check-target:
	shellcheck .devcontainer/*.sh
	python3 .devcontainer/check-vscode-settings.py

check-docs:
	platform/plugins/conventions/bin/check-docs

fmt:
	golangci-lint fmt ./...

test:
	go test -race ./...

build:
	go build -o bin/acp ./cmd/acp
