// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"errors"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
)

// SubnetAPI defines the necessary API methods needed by the subnet
// subcommands.
type SubnetAPI interface {
	io.Closer

	// AllZones returns all availability zones known to Juju.
	AllZones() ([]string, error)

	// CreateSubnet creates a new Juju subnet.
	CreateSubnet(subnetCIDR, spaceName string, zones []string, isPublic bool) error
}

var logger = loggo.GetLogger("juju.cmd.juju.subnet")

const commandDoc = `
"juju subnet" provides commands to manage Juju subnets.
`

// NewSuperCommand creates the "subnet" supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	subnetCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "subnet",
		Doc:         strings.TrimSpace(commandDoc),
		UsagePrefix: "juju",
		Purpose:     "manage subnets",
	})
	subnetCmd.Register(envcmd.Wrap(&CreateCommand{}))

	return subnetCmd
}

// SubnetCommandBase is the base type embedded into all subnet
// subcommands.
type SubnetCommandBase struct {
	envcmd.EnvCommandBase
	api SubnetAPI
}

// NewAPI returns a SubnetAPI for the root api endpoint that the
// environment command returns.
func (c *SubnetCommandBase) NewAPI() (SubnetAPI, error) {
	// TODO(dimitern): Change this once the API is implemented.

	if c.api != nil {
		// Already created.
		return c.api, nil
	}

	return nil, errors.New("API not implemented yet!")
}
