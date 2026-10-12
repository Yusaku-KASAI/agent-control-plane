#!/usr/bin/env python3
"""Checks the Markdown files in the repository against the conventions plugin's
docs/reference/formats.md and docs/architecture/documentation.md: relative links and anchors, ADR frontmatter and cross-references,
the layout of each docs/ directory, its index, and the component descriptions in architecture docs."""

import os
import re
import subprocess
import sys
import unicodedata

STATUSES = {"accepted", "superseded"}
DOCS_LAYOUT = {"README.md", "overview.md", "architecture", "guides", "reference", "adr"}
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
    return re.sub(r"`[^`\n]*`", "", FENCE.sub("", text))


def anchors(path):
    """The ids set with <a id>, and the ids GitHub derives from headings written in ASCII only."""
    text = FENCE.sub("", open(path, encoding="utf-8").read())
    ids = set(ANCHOR.findall(re.sub(r"`[^`\n]*`", "", text)))
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
            items = [v.strip() for v in value[1:-1].split(",") if v.strip()]
            meta[key.strip()] = [v.strip('"') if v.startswith('"') else int(v) for v in items]
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
        adrs[os.path.normpath(path)] = meta
    for path, meta in adrs.items():
        for key, inverse in INVERSE.items():
            for ref in meta.get(key, []):
                other = resolve_adr(path, ref)
                if other not in adrs:
                    errors.append(f"{path}: {key} {ref} does not exist")
                elif not any(resolve_adr(other, back) == path for back in adrs[other].get(inverse, [])):
                    errors.append(f"{path}: {key} {ref}, but it lacks {inverse} back to this ADR")


def resolve_adr(path, ref):
    """An ADR reference is a number in the same directory, or a relative path to another directory's ADR."""
    directory = os.path.dirname(path)
    if isinstance(ref, int):
        matches = [f for f in os.listdir(directory) if f.startswith(f"{ref:04d}-")]
        return os.path.normpath(os.path.join(directory, matches[0])) if matches else f"{directory}/{ref:04d}"
    return os.path.normpath(os.path.join(directory, ref))


def docs_roots(paths):
    return sorted({os.path.dirname(p) for p in paths if os.path.basename(os.path.dirname(p)) == "docs"
                   and os.path.basename(p) == "README.md"})


def check_docs_dirs(paths, errors):
    for root in docs_roots(paths):
        for entry in sorted(os.listdir(root)):
            if entry not in DOCS_LAYOUT:
                errors.append(f"{root}/{entry}: not part of the docs layout ({', '.join(sorted(DOCS_LAYOUT))})")
        index = os.path.join(root, "README.md")
        linked = {os.path.normpath(os.path.join(root, t)) for t, _ in LINK.findall(strip_code(open(index, encoding="utf-8").read()))}
        for path in paths:
            inside = os.path.relpath(path, root)
            if inside.startswith("..") or inside == "README.md" or inside.split(os.sep)[0] == "adr":
                continue
            if os.path.normpath(path) not in linked:
                errors.append(f"{path}: not listed in {index}")
            if inside.split(os.sep)[0] == "architecture":
                check_components(path, errors)


def check_components(path, errors):
    # Only fenced blocks are dropped: headings often consist of inline code, such as a path.
    text = FENCE.sub("", open(path, encoding="utf-8").read())
    m = re.search(r"^## 構成要素\n(.*?)(?=^## |\Z)", text, re.M | re.S)
    if not m:
        errors.append(f"{path}: no '## 構成要素' section")
        return
    for block in re.split(r"(?m)^(?=### )", m.group(1)):
        heading = re.match(r"### (.+)", block)
        if heading and "**責務**" not in block:
            errors.append(f"{path}: component '{heading.group(1)}' has no 責務")


MAP_ANCHOR = re.compile(r'<a id="map-[a-z0-9-]+"></a>')


class Map:
    """The items of the repository, gathered from every map table: name -> (parent, paths, prerequisites, table)."""

    def __init__(self, items):
        self.items = items
        self.order = []

        def visit(parent):
            for name, item in items.items():
                if item[0] == parent:
                    self.order.append(name)
                    visit(name)

        visit(None)
        self.path = next((t for _, _, _, t in items.values()), "")

    def item_of(self, file):
        best, length = None, -1
        for name, (_, paths, _, _) in self.items.items():
            for p in paths:
                if (file == p or (p.endswith("/") and file.startswith(p))) and len(p) > length:
                    best, length = name, len(p)
        return best

    def ancestors(self, name):
        chain = []
        while name is not None and name in self.items:
            chain.append(name)
            name = self.items[name][0]
        return chain

    def closure(self, name):
        seen, todo = set(), list(self.items[name][2])
        while todo:
            n = todo.pop()
            if n not in seen and n in self.items:
                seen.add(n)
                todo.extend(self.items[n][2])
        return seen


def map_tables(path):
    """The rows of each map table in a file: a table follows an <a id="map-..."> anchor."""
    text = FENCE.sub("", open(path, encoding="utf-8").read())
    tables = []
    for anchor in MAP_ANCHOR.finditer(text):
        rows = []
        for line in text[anchor.end():].splitlines()[1:]:
            if line.startswith("|"):
                rows.append([c.strip() for c in line.strip().strip("|").split("|")])
            elif rows:
                break
        tables.append(rows[2:])
    return tables


def read_map(paths, errors):
    items = {}
    for path in paths:
        for rows in map_tables(path):
            for cells in rows:
                if len(cells) != 4:
                    errors.append(f"{path}: map row needs 4 columns: {cells}")
                    continue
                name, parent, place, deps = cells
                if name in items:
                    errors.append(f"{path}: item {name} is already declared in {items[name][3]}")
                    continue
                items[name] = (parent or None, re.findall(r"`([^`]+)`", place),
                               [d.strip() for d in deps.split(",") if d.strip()], path)
    if not items:
        return None
    m = Map(items)
    roots = sorted({t for parent, _, _, t in items.values() if parent is None})
    if len(roots) != 1:
        errors.append(f"top-level items must be declared in exactly one table, found in: {', '.join(roots) or 'none'}")
    for name, (parent, _, deps, table) in items.items():
        if parent is not None and parent not in items:
            errors.append(f"{table}: item {name} has unknown parent {parent}")
            continue
        # The children of an item are mapped in the item's own documents; an item without a place of
        # its own has its children mapped where it is declared.
        if parent is not None:
            owner_paths = items[parent][1]
            if owner_paths and m.item_of(table) != parent:
                errors.append(f"{table}: item {name} belongs in a map in {parent}'s own documents")
            if not owner_paths and table != items[parent][3]:
                errors.append(f"{table}: item {name} belongs in the map that declares {parent}")
        for d in deps:
            if d not in items or items[d][0] != parent:
                errors.append(f"{table}: item {name} may only rely on its siblings, not {d}")
    for name in items:
        if name in m.closure(name):
            errors.append(f"{items[name][3]}: item {name} relies on itself through a cycle")
    return m


def check_map(m, paths, errors):
    for path in paths:
        if m.item_of(path) is None:
            errors.append(f"{path}: not in any item of the map ({m.path})")
    for path in paths:
        source = m.item_of(path)
        # An index only shows where documents are; it does not rely on them.
        if source is None or (os.path.basename(path) == "README.md" and os.path.basename(os.path.dirname(path)) == "docs"):
            continue
        for target, _ in LINK.findall(strip_code(open(path, encoding="utf-8").read())):
            if not target or re.match(r"[a-z][a-z0-9+.-]*:", target):
                continue
            resolved = os.path.normpath(os.path.join(os.path.dirname(path), target))
            # Decision records give reasons; linking to them does not rely on them.
            if "adr" in resolved.split(os.sep)[:-1]:
                continue
            dest = m.item_of(resolved)
            if dest is None or dest == source:
                continue
            a, b = m.ancestors(source), m.ancestors(dest)
            if dest in a or source in b:
                continue
            common = next(x for x in a if x in b) if set(a) & set(b) else None
            a_side = a[a.index(common) - 1] if common else a[-1]
            b_side = b[b.index(common) - 1] if common else b[-1]
            if b_side not in m.closure(a_side):
                errors.append(f"{path}: relies on {target}, but {a_side} does not rely on {b_side} in the map")


def print_map(m):
    def width(text):
        return sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in text)

    names = m.order
    label = {n: "  " * (len(m.ancestors(n)) - 1) + n for n in names}
    w = max(width(label[n]) for n in names)
    print(" " * w + " | " + " ".join(f"{i:>2}" for i in range(1, len(names) + 1)))
    for i, n in enumerate(names, 1):
        marks = ["●" if c in m.items[n][2] else ("·" if c == n else " ") for c in names]
        cells = " ".join((" " * (2 - width(x))) + x for x in marks)
        print(label[n] + " " * (w - width(label[n])) + " | " + cells + f"  {i}")

def main():
    errors = []
    paths = tracked_markdown()
    m = read_map(paths, errors)
    if "--map" in sys.argv[1:]:
        if m is None:
            print("no map table", file=sys.stderr)
            return 1
        print_map(m)
        return 0
    for path in paths:
        check_links(path, open(path, encoding="utf-8").read(), errors)
    check_adrs(paths, errors)
    check_docs_dirs(paths, errors)
    if m is not None:
        check_map(m, paths, errors)
    for e in errors:
        print(e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
