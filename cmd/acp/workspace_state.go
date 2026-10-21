package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/adapter/gitworkspace"
)

// workspaceState prints what the repository in the working directory holds. The control plane
// runs it in a container, apart from itself, since the agent may have written anything there.
func workspaceState(ctx context.Context, stdout, stderr io.Writer) int {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	state, err := gitworkspace.NewProvider("").State(ctx, dir)
	if err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(state); err != nil {
		fmt.Fprintln(stderr, "acp:", err)
		return 1
	}
	return 0
}
