.PHONY: check check-target

# One target per concern, so that CI can run each from only the files it reads.
# Fill in: this repository's check-<concern> targets, each also added to check.
check: check-target

check-target:
	shellcheck .devcontainer/*.sh
	python3 .devcontainer/check-vscode-settings.py
