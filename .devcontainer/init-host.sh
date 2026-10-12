#!/bin/sh
# Prepares the host for the devcontainer: .env (see .env.example), the state directories and the
# network shared with the control plane (per the target contract). Run it before the first start;
# values already in .env are kept.
set -eu
cd "$(dirname "$0")"
repo=$(basename "$(cd .. && pwd)")

touch .env
add() { grep -q "^$1=" .env || printf '%s=%s\n' "$1" "$2" >>.env; }
add REPO_NAME "$repo"
add HOST_UID "$(id -u)"
add HOST_GID "$(id -g)"
tz=$(readlink /etc/localtime 2>/dev/null | sed -n 's|.*/zoneinfo/||p')
add TZ "${tz:-UTC}"
add DEV_STATE_DIR "$HOME/.local/state/devcontainers/$repo"

state=$(sed -n 's/^DEV_STATE_DIR=//p' .env)
mkdir -p "$state/claude" "$state/gh" "$state/history"
docker network inspect agent-control-plane-net >/dev/null 2>&1 ||
    docker network create agent-control-plane-net >/dev/null
