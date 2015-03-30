// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"errors"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/network"
)

// SpaceAPI defines the necessary API methods needed by the space
// subcommands.
type SpaceAPI interface {
	io.Closer

	// AllSubnets returns all subnets known to Juju.
	AllSubnets() ([]network.SubnetInfo, error)

	// CreateSpace creates a new Juju network space, associating the
	// specified subnets with it (optional; can be empty).
	CreateSpace(name string, subnetIds []string) error
}

var logger = loggo.GetLogger("juju.cmd.juju.space")

const commandDoc = `
"juju space" provides commands to manage Juju network spaces.
`

// NewSuperCommand creates the "space" supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	spaceCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "space",
		Doc:         strings.TrimSpace(commandDoc),
		UsagePrefix: "juju",
		Purpose:     "manage network spaces",
	})
	spaceCmd.Register(envcmd.Wrap(&CreateCommand{}))

	return spaceCmd
}

// SpaceCommandBase is the base type embedded into all space
// subcommands.
type SpaceCommandBase struct {
	envcmd.EnvCommandBase
	api SpaceAPI
}

// NewAPI returns a SpaceAPI for the root api endpoint that the
// environment command returns.
func (c *SpaceCommandBase) NewAPI() (SpaceAPI, error) {
	// TODO(dimitern): Change this once the API is implemented.

	if c.api != nil {
		// Already created.
		return c.api, nil
	}

	return nil, errors.New("API not implemented yet!")
}
