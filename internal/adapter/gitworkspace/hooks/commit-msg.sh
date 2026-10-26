#!/bin/sh
# Adds a Co-authored-by trailer for each person in $ACP_CO_AUTHORS_FILE. The control plane installs
# it only in the workspace's .git, so it is never committed; a target that sets core.hooksPath
# bypasses it.
f="${ACP_CO_AUTHORS_FILE:-}"
[ -n "$f" ] && [ -r "$f" ] || exit 0
while IFS= read -r who; do
	[ -n "$who" ] || continue
	git interpret-trailers --in-place --if-exists addIfDifferent --trailer "Co-authored-by: $who" "$1" || exit 1
done <"$f"
