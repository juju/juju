// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju"
)

func main() {
	Main(os.Args)
}

func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(envcmd.Wrap(&restoreCommand{}), ctx, args[1:]))
}

var logger = loggo.GetLogger("juju.plugins.restore")

const restoreDoc = `
This plugin is deprecated, please use:

$ juju backups restore

The exact functionality of this command is attained by using:

$ juju backups restore -b --file <backupfile.tar.gz>
`

type restoreCommand struct {
	envcmd.EnvCommandBase
	Log             cmd.Log
	Constraints     constraints.Value
	backupFile      string
	showDescription bool
}

func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-restore",
		Purpose: "Restore a backup made with juju backup",
		Args:    "<backupfile.tar.gz>",
		Doc:     restoreDoc,
	}
}

func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set environment constraints")
	f.BoolVar(&c.showDescription, "description", false, "show the purpose of this plugin")
	c.Log.AddFlags(f)
}

func (c *restoreCommand) Init(args []string) error {
	if c.showDescription {
		return cmd.CheckEmpty(args)
	}
	if len(args) == 0 {
		return fmt.Errorf("no backup file specified")
	}
	c.backupFile = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *restoreCommand) Run(ctx *cmd.Context) error {
	if c.showDescription {
		fmt.Fprintf(ctx.Stdout, "%s\n", c.Info().Purpose)
		return nil
	}
	cmd := exec.Command("juju", "backups", "restore", "-b", "--file", c.backupFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
