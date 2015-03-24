// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.space")

const commandDoc = `
"juju space" provides commands to manage Juju network spaces.
`

// NewSuperCommand creates the "space" supercommand and registers the
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

// SpaceCommandBase is a helper base structure that has a method to get the
// space managing client.
type SpaceCommandBase struct {
	envcmd.EnvCommandBase
}
