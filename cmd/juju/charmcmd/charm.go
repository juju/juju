// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/juju/resource"
)

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
	// TODO (anastasiamac 2017-07-28) this needs to be renamed
	// to something else, for eg. 'juju charm-resources',
	// to comply with the new naming convention.
	// This command was overlooked in general rename for Juju 2.x
	charmCmd.Register(resource.NewListCharmResourcesCommand(nil))

	return charmCmd
}
