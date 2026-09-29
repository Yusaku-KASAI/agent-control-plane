#!/bin/bash
# Run by postAttachCommand as `node`; warms the Go module cache in the background.

echo "Background sync started..."

if [ -f /workspace/agent-control-plane/go.mod ]; then
    cd /workspace/agent-control-plane && go mod download
fi

echo "Background sync finished."
