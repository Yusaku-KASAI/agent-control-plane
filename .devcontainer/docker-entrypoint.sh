#!/bin/bash
# Runs as the image's user. The image is built for the owner of the workspace (HOST_UID/HOST_GID);
# say so when it was not, instead of letting tools fail on permissions later.
if [ ! -w "$PWD" ]; then
    echo "[entrypoint] $(id -u):$(id -g) cannot write $PWD, owned by $(stat -c '%u:%g' "$PWD")." \
        "Build the image with HOST_UID/HOST_GID set to the owner (see .devcontainer/.env.example)." >&2
fi
exec "$@"
