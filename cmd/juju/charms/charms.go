// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/cmd"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/envcmd"
)

var logger = loggo.GetLogger("juju.cmd.juju.charms")

const charmsCommandDoc = `
"juju charms" is used to manage the charms in the Juju environment.
`

const charmsCommandPurpose = "manage charms"

// NewSuperCommand creates the charms supercommand and registers the subcommands
// that it supports.
func NewSuperCommand() cmd.Command {
	charmscmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "charms",
		Doc:         charmsCommandDoc,
		UsagePrefix: "juju",
		Purpose:     charmsCommandPurpose,
	})
	charmscmd.Register(envcmd.Wrap(&ListCommand{}))
	return charmscmd
}

// CharmsCommandBase is a helper base structure that has a method to get the
// charms management client.
type CharmsCommandBase struct {
	envcmd.EnvCommandBase
}

// NewCharmsClient returns a charms client for the root api endpoint
// that the environment command returns.
func (c *CharmsCommandBase) NewCharmsClient() (*charms.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return charms.NewClient(root), nil
}
