// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"
)

var registeredSubCommands []cmd.Command

// RegisterSubCommand registers the given command as a "juju charm" subcommand.
func RegisterSubCommand(c cmd.Command) {
	registeredSubCommands = append(registeredSubCommands, c)
}

var charmDoc = `
"juju charm" is the the juju CLI equivalent of the "charm" command used
by charm authors, though only applicable functionality is mirrored.
`

const charmPurpose = "Interact with charms."

// Command is the top-level command wrapping all charm functionality.
type Command struct {
	cmd.SuperCommand
}

// NewSuperCommand returns a new charm super-command.
func NewSuperCommand() *Command {
	charmCmd := &Command{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "charm",
				Doc:         charmDoc,
				UsagePrefix: "juju",
				Purpose:     charmPurpose,
			},
		),
	}

	// Sub-commands may be registered directly here, like so:
	//charmCmd.Register(newXXXCommand())

	// ...or externally via RegisterSubCommand().
	for _, command := range registeredSubCommands {
		charmCmd.Register(command)
	}

	return charmCmd
}
