// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	_ "launchpad.net/juju-core/provider/all"
)

var logger = loggo.GetLogger("juju.plugins.updatebootstrap")

const updateBootstrapDoc = `
Patches all machines after state server has been restored from backup.
`

type updateBootstrapCommand struct {
	cmd.EnvCommandBase
}

func (c *updateBootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-update-bootstrap",
		Purpose: "update all machines after recovering state server",
		Doc:     updateBootstrapDoc,
	}
}

func (c *updateBootstrapCommand) Run(ctx *cmd.Context) error {
	logger.Infof("Running update-bootstrap")
	return nil
}

func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	command := updateBootstrapCommand{}
	os.Exit(cmd.Main(&command, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
