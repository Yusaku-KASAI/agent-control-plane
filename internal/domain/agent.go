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

type Agent struct {
	Name   AgentName
	Role   Role
	Target Target
}

func NewAgent(name AgentName, role Role, target Target) (Agent, error) {
	switch {
	case name == "":
		return Agent{}, errors.New("agent name is empty")
	case role == "":
		return Agent{}, fmt.Errorf("agent %q: role is empty", name)
	case !filepath.IsAbs(target.Path):
		return Agent{}, fmt.Errorf("agent %q: target path %q is not absolute", name, target.Path)
	case target.Image == "":
		return Agent{}, fmt.Errorf("agent %q: target image is empty", name)
	}
	return Agent{Name: name, Role: role, Target: target}, nil
}
