#!/bin/bash
echo "Background sync started..."

if [ -f /workspace/agent-control-plane/go.mod ]; then
    cd /workspace/agent-control-plane && go mod download
fi

echo "Background sync finished."
