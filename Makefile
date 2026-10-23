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
	platform/plugins/github/tests/prctl-test.sh
	platform/plugins/github/tests/check-protection-test.sh
	platform/plugins/github/tests/check-issues-test.sh
	platform/plugins/harness/tests/foreground-test.sh
	platform/plugins/harness/tests/detach-test.sh
	platform/plugins/harness/tests/watch-tree-test.sh
	platform/plugins/conventions/tests/check-docs-at-stop-test.sh
	platform/plugins/conventions/tests/check-docs-catalog-test.sh
	# The injected rules link their sources by path; each must exist.
	platform/plugins/conventions/scripts/context SessionStart | jq -r .hookSpecificOutput.additionalContext \
		| grep -o '](/[^)#"]*' | cut -c3- | sed 's/ *$$//' | while read -r f; do test -e "$$f" || { echo "missing: $$f"; exit 1; }; done

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
