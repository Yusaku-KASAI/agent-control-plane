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

echo "[entrypoint] Starting rsyslog (for iptables LOG capture)..." >&2
rsyslogd || echo "[entrypoint] WARNING: rsyslogd failed to start" >&2

if [ "${FIREWALL_ENABLED:-true}" = "true" ]; then
    echo "[entrypoint] Initializing firewall..." >&2
    if /usr/local/bin/init-firewall.sh >&2; then
        echo "[entrypoint] Firewall ready." >&2
    else
        echo "[entrypoint] WARNING: Firewall initialization failed. Starting without firewall." >&2
    fi
else
    echo "[entrypoint] Firewall disabled (FIREWALL_ENABLED=false)." >&2
fi

exec "$@"
