#!/bin/bash
# Docker creates named volumes root-owned, and the entrypoint may have just changed node's UID.
# Only a mount whose root is not node's is walked, so a start with nothing to fix stays fast.
set -e
owner="$(id -u node):$(id -g node)"
awk '{print $2}' /proc/self/mounts | grep -E '^(/home/node|/commandhistory)(/|$)' | sort -u |
    while read -r dir; do
        if [ "$(stat -c '%u:%g' "$dir")" != "$owner" ]; then
            chown -R node:node "$dir" 2>/dev/null || true
        fi
    done
