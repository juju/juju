package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type InitzkCommand struct {
	StateInfo  state.Info
	InstanceId string
	EnvType    string
}

// Info returns a decription of the command.
func (c *InitzkCommand) Info() *cmd.Info {
	return &cmd.Info{
		"initzk", "[options]",
		"initialize juju state in a local zookeeper",
		"",
		true,
	}
}

// Init initializes the command for running.
func (c *InitzkCommand) InitFlagSet(f *gnuflag.FlagSet, args []string) error {
	stateInfoVar(f, &c.StateInfo, "zookeeper-servers", []string{"127.0.0.1:2181"}, "address of zookeeper to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	f.StringVar(&c.EnvType, "env-type", "", "environment type")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.StateInfo.Addrs == nil {
		return requiredError("zookeeper-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if c.EnvType == "" {
		return requiredError("env-type")
	}
	return nil
}

// Run initializes zookeeper state for an environment.
func (c *InitzkCommand) Run(_ *cmd.Context) error {
	// TODO connect to zookeeper; call State.Initialize
	return fmt.Errorf("InitzkCommand.Run not implemented")
}
