package domain

import (
	"errors"
	"fmt"
	"path/filepath"
)

var ErrAgentNotFound = errors.New("agent not found")

type AgentName string

type Role string

type Target struct {
	// Path is the target repository checkout on the host running the containers.
	Path  string
	Image string
}

// GitIdentity is the author of the agent's commits. Empty fields leave it to the container.
type GitIdentity struct {
	Name  string
	Email string
}

type Agent struct {
	Name   AgentName
	Role   Role
	Target Target
	Git    GitIdentity
}

func NewAgent(name AgentName, role Role, target Target, git GitIdentity) (Agent, error) {
	switch {
	case name == "":
		return Agent{}, errors.New("agent name is empty")
	case role == "":
		return Agent{}, fmt.Errorf("agent %q: role is empty", name)
	case !filepath.IsAbs(target.Path):
		return Agent{}, fmt.Errorf("agent %q: target path %q is not absolute", name, target.Path)
	case target.Image == "":
		return Agent{}, fmt.Errorf("agent %q: target image is empty", name)
	case (git.Name == "") != (git.Email == ""):
		return Agent{}, fmt.Errorf("agent %q: git name and email must be set together", name)
	}
	return Agent{Name: name, Role: role, Target: target, Git: git}, nil
}
