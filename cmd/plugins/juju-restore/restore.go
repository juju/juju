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

	"github.com/juju/juju/cmd/modelcmd"
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
	if err := juju.InitJujuXDGDataHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(newRestoreCommand(), ctx, args[1:]))
}

var logger = loggo.GetLogger("juju.plugins.restore")

func newRestoreCommand() cmd.Command {
	return modelcmd.Wrap(&restoreCommand{})
}

const restoreDoc = `
Restore restores a backup created with juju backup
by creating a new juju bootstrap instance and arranging
it so that the existing instances in the model
talk to it.

It verifies that the existing bootstrap instance is
not running. The given constraints will be used
to choose the new instance.
`

type restoreCommand struct {
	modelcmd.ModelCommandBase
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
	f.Var(constraints.ConstraintsValue{Target: &c.Constraints}, "constraints", "set model constraints")
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
	if err := c.Log.Start(ctx); err != nil {
		return err
	}
	return c.runRestore(ctx)
}

func (c *restoreCommand) runRestore(ctx *cmd.Context) error {
	cmdArgs := []string{"backups"}
	if c.Log.Path != "" {
		cmdArgs = append(cmdArgs, "--log-file", c.Log.Path)
	}
	if c.Log.Verbose {
		cmdArgs = append(cmdArgs, "--verbose")
	}
	if c.Log.Quiet {
		cmdArgs = append(cmdArgs, "--quiet")
	}
	if c.Log.Debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if c.Log.Config != c.Log.DefaultConfig {
		cmdArgs = append(cmdArgs, "--logging-config", c.Log.Config)
	}
	if c.Log.ShowLog {
		cmdArgs = append(cmdArgs, "--show-log")
	}

	cmdArgs = append(cmdArgs, "restore", "-b", "--file", c.backupFile)
	cmd := exec.Command("juju", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
