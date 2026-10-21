package gitworkspace

import (
	"slices"
	"testing"

	"github.com/Yusaku-KASAI/agent-control-plane/internal/usecase"
)

// Whoever checks what the reading says knows the operations by usecase.Operations.
func TestOperations_AreKnown(t *testing.T) {
	for _, op := range operations {
		if !slices.Contains(usecase.Operations, op.name) {
			t.Errorf("operation %q is not in usecase.Operations", op.name)
		}
	}
}
