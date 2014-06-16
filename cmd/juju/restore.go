// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/state/api/params"
)

type restoreClient interface {
	Restore(backupFilePath string) error
}

type RestoreCommand struct {
	envcmd.EnvCommandBase
	Constraints constraints.Value
	Filename    string
}

var restoreDoc = `
Restores a backup that was previously created with "juju backup".

This command creates a new state server and arranges for it to replace
the previous state server for an environment.  It does *not* restore
an existing server to a previous state, but instead creates a new server
with equivanlent state.  As part of restore, all known instances are
configured to treat the new state server as their master.

The given constraints will be used to choose the new instance.

If the provided state cannot be restored, this command will fail with
an appropriate message.  For instance, if the existing bootstrap
instance is already running then the command will fail with a message
to that effect.
`

func (c *RestoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore",
		Purpose: "restore a backup made with juju backup",
		Args:    "<backupfile.tar.gz>",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

func (c *RestoreCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints},
		"constraints", "set environment constraints")
}

func (c *RestoreCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no backup file specified")
	}
	c.Filename = args[0]
	return cmd.CheckEmpty(args[1:])
}

const restoreAPIIncompatibility = "server version not compatible for " +
	"restore with client version"

func (c *RestoreCommand) runRestore(ctx *cmd.Context, client restoreClient) error {
	err := client.Restore(c.Filename)
	if params.IsCodeNotImplemented(err) {
		return fmt.Errorf(restoreAPIIncompatibility)
	} else if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout, "restore from %s completed\n", c.Filename)
	return nil
}

func (c *RestoreCommand) Run(ctx *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()

	return c.runRestore(ctx, client)
}
