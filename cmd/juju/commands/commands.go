// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

// TODO(ericsnow) Replace all this with a better registry mechanism,
// likely over in the cmd repo.

var registeredCommands []commandRegistryItem

func registerCommand(item commandRegistryItem) {
	registeredCommands = append(registeredCommands, item)
}

// RegisterCommand adds the provided func to the set of those that will
// be called when the juju command runs. Each returned command will be
// registered with the "juju" supercommand.
func RegisterCommand(newCommand func() cmd.Command) {
	item := commandRegistryItem{
		newCommand: newCommand,
	}
	registerCommand(item)
}

// RegisterCommand adds the provided func to the set of those that will
// be called when the juju command runs. Each returned command will be
// wrapped in envCmdWrapper, which is what gets registered with the
// "juju" supercommand.
func RegisterEnvCommand(newCommand func() envcmd.EnvironCommand) {
	item := commandRegistryItem{
		newEnvCommand: newCommand,
	}
	registerCommand(item)
}

type commandRegistryItem struct {
	newCommand    func() cmd.Command
	newEnvCommand func() envcmd.EnvironCommand
	//aliases []alias
	//deprecated bool
	//featureFlags []string
}

func (cri commandRegistryItem) command(ctx *cmd.Context) cmd.Command {
	var command cmd.Command

	switch {
	case cri.newCommand != nil:
		command = cri.newCommand()
	case cri.newEnvCommand != nil:
		envCommand := cri.newEnvCommand()
		command = envCmdWrapper{
			Command: envcmd.Wrap(envCommand),
			ctx:     ctx,
		}
	}

	return command
}

func (cri commandRegistryItem) apply(r commandRegistry, ctx *cmd.Context) {
	command := cri.command(ctx)
	r.Register(command)
}
