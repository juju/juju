package agent

import (
	"fmt"
	"launchpad.net/juju/go/state"
)

// Agent must be implemented by every juju agent.
type Agent interface {
	Run(state *state.State, jujuDir string) error
}

// Unit is a juju agent responsible for managing a single service unit.
type Unit struct {
	Name string
}

// Run runs the agent.
func (a *Unit) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("agent.Unit.Run not implemented")
}

// Machine is a juju agent responsible for managing a single machine and
// deploying service units onto it.
type Machine struct {
	Id uint
}

// Run runs the agent.
func (a *Machine) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("agent.Machine.Run not implemented")
}

// Provisioning is a juju agent responsible for launching new machines.
type Provisioning struct {
}

// Run runs the agent.
func (a *Provisioning) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("agent.Provisioning.Run not implemented")
}
