#!/bin/bash
set -e

# Single owner of UID/GID and volume permissions (docs/target/adr/0001). On Linux, re-map
# `node` to the workspace owner so the bind mount is writable; on macOS the mount reports 1000
# and this is a no-op.
WORKSPACE_DIR="${WORKSPACE_DIR:-$PWD}"
if [ -d "$WORKSPACE_DIR" ]; then
    HOST_UID=$(stat -c '%u' "$WORKSPACE_DIR")
    HOST_GID=$(stat -c '%g' "$WORKSPACE_DIR")
    if [ "$HOST_UID" != 0 ] && { [ "$HOST_UID" != "$(id -u node)" ] || [ "$HOST_GID" != "$(id -g node)" ]; }; then
        echo "[entrypoint] Aligning node UID/GID to host ${HOST_UID}:${HOST_GID}..."
        groupmod -o -g "$HOST_GID" node 2>/dev/null || true
        usermod -o -u "$HOST_UID" -g "$HOST_GID" node
        chown -R node:node /usr/local/share/npm-global 2>/dev/null || true
    fi
fi

/usr/local/bin/fix-permissions.sh || true

echo "[entrypoint] Starting rsyslog (for iptables LOG capture)..."
rsyslogd || echo "[entrypoint] WARNING: rsyslogd failed to start" >&2

if [ "${FIREWALL_ENABLED:-true}" = "true" ]; then
    echo "[entrypoint] Initializing firewall..."
    if /usr/local/bin/init-firewall.sh; then
        echo "[entrypoint] Firewall ready."
    else
        echo "[entrypoint] WARNING: Firewall initialization failed. Starting without firewall." >&2
    fi
else
    echo "[entrypoint] Firewall disabled (FIREWALL_ENABLED=false)."
fi

exec "$@"
