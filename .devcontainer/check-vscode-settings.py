#!/usr/bin/env python3
"""Fails when the VS Code settings in devcontainer.json and .vscode/settings.json differ.

Both files are JSONC, so they are compared line by line, ignoring indentation and whole-line comments.
"""

import sys
from pathlib import Path

root = Path(__file__).resolve().parent.parent


def settings_lines(text: str) -> list[str]:
    lines = [line.strip() for line in text.splitlines()]
    return [line for line in lines if line and not line.startswith("//")]


def block(lines: list[str], opening: str) -> list[str]:
    """Returns the lines inside the object that opens on the line equal to opening."""
    start = lines.index(opening)
    depth = 0
    for i in range(start, len(lines)):
        depth += lines[i].count("{") - lines[i].count("}")
        if depth == 0:
            return lines[start + 1 : i]
    raise ValueError(f"{opening} is not closed")


devcontainer = settings_lines((root / ".devcontainer/devcontainer.json").read_text())
vscode = settings_lines((root / ".vscode/settings.json").read_text())
inner = block(devcontainer, '"settings": {')
# The last setting has no trailing comma in either file, so the lists compare directly.
if inner != vscode[1:-1]:
    for a, b in zip(inner, vscode[1:-1]):
        if a != b:
            print(f"devcontainer.json: {a}\n.vscode/settings.json: {b}", file=sys.stderr)
            break
    sys.exit("VS Code settings in .devcontainer/devcontainer.json and .vscode/settings.json differ")
