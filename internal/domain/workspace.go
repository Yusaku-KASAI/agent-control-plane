package domain

// Workspace is a per-run copy of the target repository that the agent may modify freely.
type Workspace struct {
	// Path is on the host running the containers.
	Path string
}
