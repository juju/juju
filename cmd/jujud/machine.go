package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
	"strconv"
)

type MachineCommand struct {
	conf      *AgentConf
	machineId string
	MachineId uint
}

func NewMachineCommand() *MachineCommand {
	return &MachineCommand{conf: NewAgentConf()}
}

// Info returns a decription of the command.
func (c *MachineCommand) Info() *cmd.Info {
	return &cmd.Info{"machine", "[options]", "run a juju machine agent", "", true}
}

// InitFlagSet prepares a FlagSet.
func (c *MachineCommand) InitFlagSet(f *gnuflag.FlagSet) {
	c.conf.InitFlagSet(f)
	f.StringVar(&c.machineId, "machine-id", c.machineId, "id of the machine to run")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *MachineCommand) ParsePositional(args []string) error {
	if err := c.conf.Validate(); err != nil {
		return err
	}
	if c.machineId == "" {
		return requiredError("machine-id")
	}
	id, err := strconv.ParseUint(c.machineId, 10, 0)
	if err != nil {
		return fmt.Errorf("--machine-id option expects a non-negative integer")
	}
	c.MachineId = uint(id)
	return cmd.CheckEmpty(args)
}

// Run runs a machine agent.
func (c *MachineCommand) Run() error {
	return c.conf.Run(&MachineAgent{Id: c.MachineId})
}

// MachineAgent is responsible for managing a single machine and
// deploying service units onto it.
type MachineAgent struct {
	Id uint
}

// Run runs the agent.
func (a *MachineAgent) Run(state *state.State, jujuDir string) error {
	return fmt.Errorf("MachineAgent.Run not implemented")
}
