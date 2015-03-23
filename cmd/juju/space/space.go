// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.spaces")

const commandDoc = `
"juju spaces" provides commands to interact with Juju network spaces.
`

// NewSuperCommand creates the environment supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	spaceCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "space",
		Doc:         commandDoc,
		UsagePrefix: "juju",
		Purpose:     "manage network spaces",
	})
	spaceCmd.Register(envcmd.Wrap(&CreateCommand{}))

	return spaceCmd
}
