#!/usr/bin/env python3
"""Checks the Markdown files in the repository against the conventions plugin's docs/reference/formats.md
and docs/architecture/documentation.md: links and anchors, reachability from README.md and CLAUDE.md, ADR
frontmatter and cross-references, the layout of each docs/ directory and its index, component descriptions
in architecture docs, and the maps of items with the prerequisite directions between them. With --map, prints the maps as a design structure matrix."""

import os
import re
import subprocess
import sys
import unicodedata

STATUSES = {"accepted", "superseded"}
DOCS_LAYOUT = {"README.md", "overview.md", "architecture", "guides", "reference", "adr"}
# Each key names other ADRs; the value is the key those ADRs must answer with.
INVERSE = {"amends": "amended_by", "amended_by": "amends", "supersedes": "superseded_by", "superseded_by": "supersedes"}
# A link titled so only shows where something is; it does not rely on it.
NAVIGATION = "案内"
TITLE = r'''(?:\s+(?:"([^"]*)"|'([^']*)'))?'''
INLINE_LINK = re.compile(r"\]\(\s*<?([^)\s#>]*)(?:#([^)\s>]*))?>?" + TITLE + r"\s*\)")
LINK_DEFINITION = re.compile(r"^\s{0,3}\[[^\]]+\]:\s*<?([^\s#>]*)(?:#([^\s>]*))?>?" + TITLE + r"\s*$", re.M)
ANCHOR = re.compile(r'<a id="([^"]+)"></a>')
MAP_ANCHOR = re.compile(r'<a id="map-[a-z0-9-]+"></a>')
MAP_COLUMNS = ["項目", "親", "置き場所", "前提にする"]
FENCE = re.compile(r"^ {0,3}(```|~~~).*?^ {0,3}\1", re.M | re.S)
WHOLE = "全体"
# Files the OS leaves in directories; they are not documents.
IGNORED_ENTRIES = {".DS_Store"}


def tracked_markdown():
    try:
        out = subprocess.run(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "*.md"],
                             capture_output=True, text=True, check=True).stdout
        return sorted(p for p in out.split("\0") if p and os.path.exists(p))
    except (OSError, subprocess.CalledProcessError):
        # The CI build context has no .git; it holds only what the dockerignore lets in.
        paths = []
        for directory, dirs, files in os.walk("."):
            dirs[:] = [d for d in dirs if d not in {".git", "node_modules"}]
            paths += [os.path.relpath(os.path.join(directory, f)) for f in files if f.endswith(".md")]
        return sorted(paths)


def read(path):
    with open(path, encoding="utf-8") as f:
        return f.read()


def without_fences(text):
    return FENCE.sub("", text)


def strip_code(text):
    return re.sub(r"`[^`\n]*`", "", without_fences(text))


def links(text):
    """(target, fragment, title) of every inline link and link reference definition outside code."""
    text = strip_code(text)
    for m in list(INLINE_LINK.finditer(text)) + list(LINK_DEFINITION.finditer(text)):
        target, fragment, double, single = m.groups()
        yield target, fragment or "", double or single or ""


def repo_root():
    try:
        return subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True,
                              check=True).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        # Without .git (the CI build context), the script still sits in tools/ of the repository.
        return os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def is_external(target):
    return bool(re.match(r"[a-z][a-z0-9+.-]*:", target))


def resolve(path, target):
    resolved = os.path.normpath(os.path.join(os.path.dirname(path), target)) if target else path
    return resolved + "/" if target.endswith("/") or os.path.isdir(resolved) else resolved


def is_adr(path):
    return "adr" in path.rstrip("/").split(os.sep)


def is_index(path):
    return os.path.basename(path) == "README.md" and os.path.basename(os.path.dirname(path)) == "docs"


def anchors(path):
    """The ids set with <a id>, and the ids GitHub derives from headings written in ASCII only."""
    text = without_fences(read(path))
    ids = set(ANCHOR.findall(re.sub(r"`[^`\n]*`", "", text)))
    seen = {}
    for heading in re.findall(r"^#{1,6} (.+)$", text, re.M):
        if heading.isascii():
            slug = re.sub(r"[^a-z0-9 _-]", "", heading.strip().lower()).replace(" ", "-")
            ids.add(slug if slug not in seen else f"{slug}-{seen[slug]}")
            seen[slug] = seen.get(slug, 0) + 1
    return ids


def check_links(path, errors):
    for target, fragment, _ in links(read(path)):
        if is_external(target):
            continue
        if target.startswith("/"):
            errors.append(f"{path}: absolute link {target}; link with a relative path")
            continue
        resolved = resolve(path, target).rstrip("/")
        if not os.path.exists(resolved):
            errors.append(f"{path}: broken link {target}")
        elif fragment and resolved.endswith(".md"):
            if not fragment.isascii():
                errors.append(f"{path}: link #{fragment} with an <a id> instead of a non-ASCII heading")
            elif fragment not in anchors(resolved):
                errors.append(f"{path}: no anchor #{fragment} in {target or path}")


def parse_frontmatter(path, text, errors):
    m = re.match(r"---\n(.*?)\n---\n", text, re.S)
    if not m:
        return None
    meta = {}
    for line in m.group(1).splitlines():
        key, _, value = line.partition(":")
        key, value = key.strip(), re.sub(r"\s+#.*$", "", value).strip()
        if key in INVERSE:
            if not (value.startswith("[") and value.endswith("]")):
                errors.append(f"{path}: {key} must be a list such as [3] or [\"../x/adr/0001-y.md\"]")
                continue
            refs = []
            for item in (v.strip() for v in value[1:-1].split(",") if v.strip()):
                if re.fullmatch(r"\d+", item):
                    refs.append(int(item))
                elif re.fullmatch(r'"[^"]+"', item):
                    refs.append(item[1:-1])
                else:
                    errors.append(f"{path}: {key} entry {item} must be a number or a quoted path")
            meta[key] = refs
        else:
            meta[key] = value
    return meta


def check_adrs(paths, errors):
    adrs, numbers = {}, {}
    for path in paths:
        if not is_adr(os.path.dirname(path)):
            continue
        name = os.path.basename(path)
        if not re.match(r"\d{4}-[a-z0-9-]+\.md$", name):
            errors.append(f"{path}: an ADR file is named NNNN-<slug>.md")
            continue
        key = (os.path.dirname(path), name[:4])
        if key in numbers:
            errors.append(f"{path}: number {name[:4]} is also used by {numbers[key]}")
        numbers[key] = path
        text = read(path)
        meta = parse_frontmatter(path, text, errors)
        if meta is None:
            errors.append(f"{path}: no frontmatter")
            continue
        for k in meta.keys() - {"status", "date", *INVERSE}:
            errors.append(f"{path}: unknown key {k}")
        if meta.get("status") not in STATUSES:
            errors.append(f"{path}: status must be one of {sorted(STATUSES)}")
        if (meta.get("status") == "superseded") != bool(meta.get("superseded_by")):
            errors.append(f"{path}: status superseded goes with superseded_by")
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", str(meta.get("date", ""))):
            errors.append(f"{path}: date must be YYYY-MM-DD")
        first_heading = re.search(r"^# .*$", text[text.index("\n---\n", 3) + 5:], re.M)
        if not first_heading or not first_heading.group(0).startswith(f"# {name[:4]}. "):
            errors.append(f"{path}: the title must be the first heading and start with '# {name[:4]}. '")
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
        matches = sorted(f for f in os.listdir(directory) if f.startswith(f"{ref:04d}-"))
        return os.path.normpath(os.path.join(directory, matches[0])) if matches else f"{directory}/{ref:04d}"
    return os.path.normpath(os.path.join(directory, ref))


def docs_roots(paths):
    return sorted({os.path.dirname(p) for p in paths if is_index(p)})


def check_docs_dirs(paths, errors):
    for root in docs_roots(paths):
        for entry in sorted(os.listdir(root)):
            if entry not in DOCS_LAYOUT and entry not in IGNORED_ENTRIES:
                errors.append(f"{root}/{entry}: not part of the docs layout ({', '.join(sorted(DOCS_LAYOUT))})")
        index = os.path.join(root, "README.md")
        linked = {resolve(index, t).rstrip("/") for t, _, _ in links(read(index)) if not is_external(t)}
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
    text = without_fences(read(path))
    m = re.search(r"^## 構成要素\n(.*?)(?=^## |\Z)", text, re.M | re.S)
    if not m:
        errors.append(f"{path}: no '## 構成要素' section")
        return
    for block in re.split(r"(?m)^(?=### )", m.group(1)):
        heading = re.match(r"### (.+)", block)
        if heading and "**責務**" not in block:
            errors.append(f"{path}: component '{heading.group(1)}' has no 責務")


# Documents read at every start, and how many lines (without blank lines and frontmatter) each may hold.
def in_plugin_dir(path, name):
    """Whether the path is directly in the given directory of a plugin (a directory with .claude-plugin/plugin.json)."""
    directory = os.path.dirname(path)
    return (os.path.basename(directory) == name
            and os.path.isfile(os.path.join(os.path.dirname(directory), ".claude-plugin", "plugin.json")))


def in_project_agents(path):
    return path.split(os.sep)[-3:-1] == [".claude", "agents"]


ALWAYS_READ = [
    (lambda p: p == "CLAUDE.md", 60, "file"),
    (lambda p: in_plugin_dir(p, "rules"), 40, "directory"),
    (lambda p: in_plugin_dir(p, "agents") or in_project_agents(p), 50, "file"),
]


def content_lines(path):
    text = read(path)
    m = re.match(r"---\n.*?\n---\n", text, re.S)
    body = text[m.end():] if m else text
    return sum(1 for line in body.splitlines() if line.strip())


def check_sizes(paths, errors):
    for matches, limit, unit in ALWAYS_READ:
        groups = {}
        for path in paths:
            if matches(path):
                key = os.path.dirname(path) if unit == "directory" else path
                groups[key] = groups.get(key, 0) + content_lines(path)
        for key, lines in sorted(groups.items()):
            if lines > limit:
                errors.append(f"{key}: {lines} lines, more than the {limit} lines a document read at every start may hold")


class Map:
    """The items gathered from every map table: name -> parent, places, prerequisites, and the table's file."""

    def __init__(self, items):
        self.items = items
        self.order = []

        def visit(parent):
            for name, item in items.items():
                if item["parent"] == parent:
                    self.order.append(name)
                    visit(name)

        visit(None)

    def item_of(self, path):
        best, length = None, -1
        for name, item in self.items.items():
            for place in item["places"]:
                hit = path == place or path == place.rstrip("/") or (place.endswith("/") and path.startswith(place))
                if hit and len(place) > length:
                    best, length = name, len(place)
        return best

    def ancestors(self, name):
        chain = []
        while name is not None and name in self.items and name not in chain:
            chain.append(name)
            name = self.items[name]["parent"]
        return chain

    def closure(self, name):
        seen, todo = set(), list(self.items[name]["deps"])
        while todo:
            n = todo.pop()
            if n not in seen and n in self.items:
                seen.add(n)
                todo.extend(self.items[n]["deps"])
        return seen

    def children(self, name):
        return [n for n, item in self.items.items() if item["parent"] == name]

    def may_rely(self, source, dest):
        """Whether a document of item source may rely on one of item dest. An item that has children holds
        its own documents as an implicit item WHOLE beside them: every child relies on it, and it relies on
        no child, so a parent only shows where its children are."""
        if source == dest:
            return True
        a, b = self.ancestors(source), self.ancestors(dest)
        if dest in a:
            return True
        if source in b:
            return False
        common = next((x for x in a if x in b), None)
        a_side = a[a.index(common) - 1] if common else a[-1]
        b_side = b[b.index(common) - 1] if common else b[-1]
        return b_side in self.closure(a_side)


def map_tables(path, errors):
    """The rows of each map table in a file. A table belongs to the <a id="map-..."> before it; a heading
    and text may come between them, but not another heading."""
    text = without_fences(read(path))
    code_free = re.sub(r"`[^`\n]*`", lambda m: " " * len(m.group(0)), text)
    tables = []
    for anchor in MAP_ANCHOR.finditer(code_free):
        rows, headings = [], 0
        for line in text[anchor.end():].splitlines()[1:]:
            if line.startswith("|"):
                rows.append([c.strip() for c in line.strip().strip("|").split("|")])
            elif rows:
                break
            elif line.startswith("#"):
                headings += 1
                if headings > 1:
                    break
        if len(rows) < 2 or rows[0][:4] != MAP_COLUMNS:
            errors.append(f"{path}: the map after {anchor.group(0)} needs a table with the columns {' | '.join(MAP_COLUMNS)}")
            continue
        tables.append(rows[2:])
    return tables


def read_map(paths, errors):
    items = {}
    for path in paths:
        for rows in map_tables(path, errors):
            for cells in rows:
                name, parent, place, deps = (cells + ["", "", "", ""])[:4]
                if name in items:
                    errors.append(f"{path}: item {name} is already declared in {items[name]['table']}")
                    continue
                places = []
                for p in re.findall(r"`([^`]+)`", place):
                    resolved = os.path.normpath(os.path.join(os.path.dirname(path), p))
                    if not os.path.exists(resolved):
                        errors.append(f"{path}: place {p} of item {name} does not exist")
                    places.append(resolved + "/" if p.endswith("/") else resolved)
                items[name] = {"parent": parent or None, "places": places, "table": path,
                               "deps": [d.strip() for d in deps.split(",") if d.strip()]}
    if not items:
        return None
    m = Map(items)
    roots = sorted({item["table"] for item in items.values() if item["parent"] is None})
    if len(roots) != 1:
        errors.append(f"top-level items must be declared in exactly one table, found in: {', '.join(roots) or 'none'}")
    for name, item in items.items():
        parent, table = item["parent"], item["table"]
        if parent is not None and parent not in items:
            errors.append(f"{table}: item {name} has unknown parent {parent}")
            continue
        if name in m.ancestors(parent):
            errors.append(f"{table}: item {name} is its own ancestor")
            continue
        # The items inside an item are mapped in its own documents; an item without a place of its own
        # has the items inside it mapped where it is declared.
        if parent is not None:
            if items[parent]["places"] and m.item_of(table) != parent:
                errors.append(f"{table}: item {name} belongs in a map in {parent}'s own documents")
            if not items[parent]["places"] and table != items[parent]["table"]:
                errors.append(f"{table}: item {name} belongs in the map that declares {parent}")
        for d in item["deps"]:
            if d not in items or items[d]["parent"] != parent:
                errors.append(f"{table}: item {name} may only rely on its siblings, not {d}")
    for name, item in items.items():
        if name in m.closure(name):
            errors.append(f"{item['table']}: item {name} relies on itself through a cycle")
    return m


def check_map(m, paths, errors):
    for path in paths:
        if m.item_of(path) is None:
            errors.append(f"{path}: not in any item of the maps")
    for path in paths:
        source = m.item_of(path)
        # An index only shows where documents are, and a decision record is not rewritten.
        if source is None or is_index(path) or is_adr(os.path.dirname(path)):
            continue
        for target, _, title in links(read(path)):
            if not target or title == NAVIGATION or is_external(target):
                continue
            resolved = resolve(path, target)
            # Decision records give reasons; linking to them does not rely on them.
            if is_adr(resolved):
                continue
            dest = m.item_of(resolved)
            if dest is not None and not m.may_rely(source, dest):
                errors.append(f"{path}: relies on {target}, but {source} may not rely on {dest} in the maps"
                              " (a link that only shows where something is takes the title \"案内\")")


# Where people and agents start reading: every document must be reachable by links from each of them.
ENTRIES = ["README.md", "CLAUDE.md"]


def reachable(start, paths):
    """The documents reached by following links from start. A link to a directory reaches every document in it."""
    seen, stack = set(), [start]
    while stack:
        path = stack.pop()
        if path in seen:
            continue
        seen.add(path)
        for target, _, _ in links(read(path)):
            if is_external(target) or target.startswith("/"):
                continue
            resolved = resolve(path, target).rstrip("/")
            if os.path.isdir(resolved):
                stack += [p for p in paths if p.startswith(resolved + os.sep) or resolved == "."]
            elif resolved in paths:
                stack.append(resolved)
    return seen


def check_reachable(paths, errors):
    found = set(paths)
    for entry in ENTRIES:
        if entry not in found:
            continue
        for path in sorted(found - reachable(entry, found) - set(ENTRIES)):
            errors.append(f"{path}: not reachable by links from {entry}")


def print_map(m):
    def width(text):
        return sum(2 if unicodedata.east_asian_width(c) in "WF" else 1 for c in text)

    names = m.order
    label = {n: "  " * (len(m.ancestors(n)) - 1) + n for n in names}
    w = max(width(label[n]) for n in names)
    print(" " * w + " | " + " ".join(f"{i:>2}" for i in range(1, len(names) + 1)))
    for i, n in enumerate(names, 1):
        marks = ["●" if c in m.items[n]["deps"] else ("·" if c == n else " ") for c in names]
        print(label[n] + " " * (w - width(label[n])) + " | " + " ".join(f"{x:>2}" for x in marks) + f"  {i}")
    print(f"\n● 前提にする（行 → 列）。子を持つ項目の直下の文書は、子と並ぶ「{WHOLE}」として、どの子からも前提にされ、どの子も前提にしない。")


def main():
    os.chdir(repo_root())
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
        check_links(path, errors)
    check_adrs(paths, errors)
    check_docs_dirs(paths, errors)
    check_sizes(paths, errors)
    check_reachable(paths, errors)
    if m is not None:
        check_map(m, paths, errors)
    for e in errors:
        print(e, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
