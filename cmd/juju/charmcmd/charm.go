// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"
)

var charmDoc = `
"juju charm" is the the juju CLI equivalent of the "charm" command used
by charm authors, though only applicable functionality is mirrored.
`

const charmPurpose = "interact with charms"

// Command is the top-level command wrapping all backups functionality.
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
	//charmCmd.Register(newXXXCommand())
	return charmCmd
}
