// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package server

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/serveradmin"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.server")

// SuperCommand is the top level server command that has subcommands to operate
// on the Juju server.
type SuperCommand struct {
	*cmd.SuperCommand
}

const serverCommandDoc = `
"juju server" is used for administration tasks on the Juju server that manages
environments and their state.
`

const serverCommandPurpose = "Juju server administration"

// NewSuperCommand creates the server supercommand and registers its subcommands.
func NewSuperCommand() cmd.Command {
	serverCmd := &SuperCommand{
		SuperCommand: cmd.NewSuperCommand(cmd.SuperCommandParams{
			Name:        "server",
			Doc:         serverCommandDoc,
			UsagePrefix: "juju",
			Purpose:     serverCommandPurpose,
		}),
	}
	serverCmd.Register(envcmd.Wrap(&TrustCommand{}))
	return serverCmd
}

// CommandBase defines some common functionality for all server commands.
type CommandBase struct {
	envcmd.EnvCommandBase
	api AdminAPI
}

// NewAdminClient returns a serveradmin client for the root api endpoint
// that the environment command returns.
func (c *CommandBase) NewAdminClient() (*serveradmin.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return serveradmin.NewClient(root), nil
}

// AdminAPI defines the serveradmin API methods used by the server commands.
type AdminAPI interface {

	// IdentityProvider returns the identity provider trusted by the Juju state
	// server, if any.
	IdentityProvider() (*params.IdentityProviderInfo, error)

	// SetIdentityProvider sets the identity provider public key and location
	// that the Juju state server should trust.
	SetIdentityProvider(publicKey, location string) error

	// Close closes the API connection.
	Close() error
}
