.PHONY: check check-go check-platform check-target check-docs fmt test build

# One target per concern, so that CI can run each from only the files it reads.
check: check-go check-platform check-target check-docs

check-go:
	golangci-lint run ./...
	go mod tidy -diff
	govulncheck ./...
	go test -race ./...

check-platform:
	claude plugin validate ./platform
	shellcheck $(wildcard platform/plugins/*/bin/* platform/plugins/*/scripts/* platform/plugins/*/tests/*)
	platform/plugins/github/tests/prctl-test.sh
	# The injected rules link their sources by path; each must exist.
	platform/plugins/conventions/scripts/context SessionStart | jq -r .hookSpecificOutput.additionalContext \
		| grep -o '](/[^)#"]*' | cut -c3- | sed 's/ *$$//' | while read -r f; do test -e "$$f" || { echo "missing: $$f"; exit 1; }; done

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
