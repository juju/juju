package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
)

type InitzkCommand struct {
	Zookeeper    string
	InstanceId   string
	ProviderType string
}

// Info returns a decription of the command.
func (c *InitzkCommand) Info() *cmd.Info {
	return cmd.NewInfo(
		"initzk", "[options]",
		"initialize juju state in a local zookeeper",
		"",
	)
}

// InitFlagSet prepares a FlagSet.
func (c *InitzkCommand) InitFlagSet(f *gnuflag.FlagSet) {
	f.StringVar(&c.Zookeeper, "zookeeper-servers", "127.0.0.1:2181", "address of zookeeper to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	f.StringVar(&c.ProviderType, "provider-type", "", "envionment machine provider type")
}

// ParsePositional checks that there are no unwanted arguments, and that all
// required flags have been set.
func (c *InitzkCommand) ParsePositional(args []string) error {
	if c.Zookeeper == "" {
		return requiredError("zookeeper-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if c.ProviderType == "" {
		return requiredError("provider-type")
	}
	return cmd.CheckEmpty(args)
}

// Run initializes zookeeper state for an environment.
func (c *InitzkCommand) Run() error {
	// TODO connect once new state.Open is available
	// (note, Zookeeper will likely need to become some sort of StateInfo)
	// state, err := state.Open(c.Zookeeper)
	// if err != nil {
	//     return err
	// }
	// defer state.Close()
	// return state.Initialize(c.InstanceId, c.ProviderType)
	return fmt.Errorf("InitzkCommand.Run not implemented")
}
