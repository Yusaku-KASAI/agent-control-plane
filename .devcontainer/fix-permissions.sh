#!/bin/bash
# Docker creates named volumes root-owned; hand them to `node`. Run by docker-entrypoint.sh.
set -e
chown -R node:node \
    /home/node \
    /commandhistory 2>/dev/null || true
