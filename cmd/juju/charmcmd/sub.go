// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/charmstore"
)

var registeredSubCommands []func(CharmstoreSpec) cmd.Command

// RegisterSubCommand adds the provided func to the set of those that
// will be called when the juju command runs. Each returned command will
// be registered with the identified "juju" sub-supercommand.
func RegisterSubCommand(newCommand func(CharmstoreSpec) cmd.Command) {
	registeredSubCommands = append(registeredSubCommands, newCommand)
}

// NewCommandBase returns a new CommandBase.
func NewCommandBase(spec CharmstoreSpec) *CommandBase {
	return &CommandBase{
		spec: newCharmstoreSpec(),
	}
}

// CommandBase is the type that should be embedded in "juju charm"
// sub-commands.
type CommandBase struct {
	// TODO(ericsnow) This should be a modelcmd.ModelCommandBase.
	cmd.CommandBase
	spec CharmstoreSpec
}

// Connect implements CommandBase.
func (c *CommandBase) Connect(ctx *cmd.Context) (*charmstore.Client, error) {
	if c.spec == nil {
		return nil, errors.Errorf("missing charm store spec")
	}
	client, err := c.spec.Connect(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return client, nil
}
