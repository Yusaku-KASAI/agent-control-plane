#!/bin/bash
set -e

# Single owner of UID/GID and volume permissions (docs/target/adr/0001).
WORKSPACE_DIR="${WORKSPACE_DIR:-$PWD}"
if [ -d "$WORKSPACE_DIR" ]; then
    HOST_UID=$(stat -c '%u' "$WORKSPACE_DIR")
    HOST_GID=$(stat -c '%g' "$WORKSPACE_DIR")
    if [ "$HOST_UID" != 0 ] && { [ "$HOST_UID" != "$(id -u node)" ] || [ "$HOST_GID" != "$(id -g node)" ]; }; then
        echo "[entrypoint] Aligning node UID/GID to host ${HOST_UID}:${HOST_GID}..." >&2
        groupmod -o -g "$HOST_GID" node 2>/dev/null || true
        usermod -o -u "$HOST_UID" -g "$HOST_GID" node
        chown -R node:node /usr/local/share/npm-global 2>/dev/null || true
    fi
fi

/usr/local/bin/fix-permissions.sh || true

exec "$@"
