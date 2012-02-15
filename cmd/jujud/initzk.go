package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

type InitzkCommand struct {
	ZookeeperAddrs []string
	InstanceId     string
	EnvType        string
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

// InitFlagSet prepares a FlagSet.
func (c *InitzkCommand) InitFlagSet(f *gnuflag.FlagSet) {
	zkAddrsVar(f, &c.ZookeeperAddrs, "zookeeper-servers", []string{"127.0.0.1:2181"}, "address of zookeeper to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	f.StringVar(&c.EnvType, "env-type", "", "environment type")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *InitzkCommand) ParsePositional(args []string) error {
	if c.ZookeeperAddrs == nil {
		return requiredError("zookeeper-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if c.EnvType == "" {
		return requiredError("env-type")
	}
	return cmd.CheckEmpty(args)
}

// Run initializes zookeeper state for an environment.
func (c *InitzkCommand) Run() error {
	// TODO connect to zookeeper; call State.Initialize
	return fmt.Errorf("InitzkCommand.Run not implemented")
}
