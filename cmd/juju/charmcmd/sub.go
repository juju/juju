// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

var registeredSubCommands []func(CharmstoreSpec) CommandBase

// RegisterSubCommand adds the provided func to the set of those that
// will be called when the juju command runs. Each returned command will
// be registered with the identified "juju" sub-supercommand.
func RegisterSubCommand(newCommand func(CharmstoreSpec) CommandBase) {
	registeredSubCommands = append(registeredSubCommands, newCommand)
}

// CommandBase is the type that should be embedded in "juju charm"
// sub-commands.
type CommandBase interface {
	io.Closer
	cmd.Command

	// Connect connects to the charm store and returns a client.
	Connect() (CharmstoreClient, error)
}

// NewCommandBase returns a new CommandBase.
func NewCommandBase(spec CharmstoreSpec) CommandBase {
	return &commandBase{
		spec: newCharmstoreSpec(),
	}
}

type commandBase struct {
	cmd.Command
	spec   CharmstoreSpec
	client CharmstoreClient
}

// Connect implements CommandBase.
func (c *commandBase) Connect() (CharmstoreClient, error) {
	if c.client != nil {
		return c.client, nil
	}

	if c.spec == nil {
		return nil, errors.Errorf("missing charm store spec")
	}
	client, err := c.spec.Connect()
	if err != nil {
		return nil, errors.Trace(err)
	}

	c.client = client
	return client, nil
}

// Close implements CommandBase.
func (c *commandBase) Close() error {
	if c.client == nil {
		return nil
	}

	if err := c.client.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func wrapCommand(command CommandBase) cmd.Command {
	return &commandWrapper{command}
}

type commandWrapper struct {
	CommandBase
}

// Run implements cmd.Command.
func (w *commandWrapper) Run(ctx *cmd.Context) error {
	defer w.Close() // Any error is discarded...
	return w.CommandBase.Run(ctx)
}
