#!/usr/bin/env python3
"""Checks the Markdown files in the repository: relative links point at existing files, and ADRs keep
the frontmatter and cross-references described in the platform's docs/reference/formats.md."""

import os
import re
import subprocess
import sys

STATUSES = {"accepted", "superseded"}
# Each key names ADRs in the same directory; the value is the key the other ADR must answer with.
INVERSE = {"amends": "amended_by", "amended_by": "amends", "supersedes": "superseded_by", "superseded_by": "supersedes"}
LINK = re.compile(r"\]\(([^)\s#]*)(?:#([^)\s]*))?\)")
ANCHOR = re.compile(r'<a id="([^"]+)"></a>')
FENCE = re.compile(r"^(```|~~~).*?^\1", re.M | re.S)


def tracked_markdown():
    try:
        out = subprocess.run(["git", "ls-files", "--cached", "--others", "--exclude-standard", "*.md"],
                             capture_output=True, text=True, check=True).stdout
        return [p for p in out.split() if os.path.exists(p)]
    except (OSError, subprocess.CalledProcessError):
        # The CI build context has no .git; it holds only what the dockerignore lets in.
        paths = []
        for directory, dirs, files in os.walk("."):
            dirs[:] = [d for d in dirs if d not in {".git", "node_modules"}]
            paths += [os.path.relpath(os.path.join(directory, f)) for f in files if f.endswith(".md")]
        return sorted(paths)


def strip_code(text):
    return FENCE.sub("", re.sub(r"`[^`\n]*`", "", text))


def anchors(path):
    """The ids set with <a id>, and the ids GitHub derives from headings written in ASCII only."""
    text = strip_code(open(path, encoding="utf-8").read())
    ids = set(ANCHOR.findall(text))
    for heading in re.findall(r"^#{1,6} (.+)$", text, re.M):
        if heading.isascii():
            ids.add(re.sub(r"[^a-z0-9 _-]", "", heading.strip().lower()).replace(" ", "-"))
    return ids


def check_links(path, text, errors):
    for target, fragment in LINK.findall(strip_code(text)):
        if re.match(r"[a-z][a-z0-9+.-]*:", target):
            continue
        resolved = os.path.normpath(os.path.join(os.path.dirname(path), target)) if target else path
        if not os.path.exists(resolved):
            errors.append(f"{path}: broken link {target}")
        elif fragment and resolved.endswith(".md"):
            if not fragment.isascii():
                errors.append(f"{path}: link #{fragment} with an <a id> instead of a non-ASCII heading")
            elif fragment not in anchors(resolved):
                errors.append(f"{path}: no anchor #{fragment} in {target or path}")


def parse_frontmatter(text):
    m = re.match(r"---\n(.*?)\n---\n", text, re.S)
    if not m:
        return None
    meta = {}
    for line in m.group(1).splitlines():
        key, _, value = line.partition(":")
        value = value.strip()
        if value.startswith("[") and value.endswith("]"):
            meta[key.strip()] = [int(v) for v in value[1:-1].split(",") if v.strip()]
        else:
            meta[key.strip()] = value
    return meta


def check_adrs(paths, errors):
    adrs = {}
    for path in paths:
        name = os.path.basename(path)
        if "adr" not in path.split(os.sep)[:-1] or not re.match(r"\d{4}-", name):
            continue
        number = int(name[:4])
        text = open(path, encoding="utf-8").read()
        meta = parse_frontmatter(text)
        if meta is None:
            errors.append(f"{path}: no frontmatter")
            continue
        for key in meta.keys() - {"status", "date", *INVERSE}:
            errors.append(f"{path}: unknown key {key}")
        if meta.get("status") not in STATUSES:
            errors.append(f"{path}: status must be one of {sorted(STATUSES)}")
        if (meta.get("status") == "superseded") != bool(meta.get("superseded_by")):
            errors.append(f"{path}: status superseded goes with superseded_by")
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", str(meta.get("date", ""))):
            errors.append(f"{path}: date must be YYYY-MM-DD")
        if not re.search(rf"^# {name[:4]}\. ", text, re.M):
            errors.append(f"{path}: heading must start with '# {name[:4]}. '")
        adrs[(os.path.dirname(path), number)] = (path, meta)
    for (directory, number), (path, meta) in adrs.items():
        for key, inverse in INVERSE.items():
            for other in meta.get(key, []):
                if (directory, other) not in adrs:
                    errors.append(f"{path}: {key} {other:04d} does not exist")
                elif number not in adrs[(directory, other)][1].get(inverse, []):
                    errors.append(f"{path}: {key} {other:04d}, but {other:04d} lacks {inverse} {number:04d}")


def main():
    errors = []
    paths = tracked_markdown()
    for path in paths:
        check_links(path, open(path, encoding="utf-8").read(), errors)
    check_adrs(paths, errors)
    for e in errors:
        print(e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
