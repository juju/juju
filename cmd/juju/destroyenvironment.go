// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	EnvCommandBase
	assumeYes bool
}

func (c *DestroyEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-environment",
		Purpose: "terminate all machines and other associated resources for an environment",
	}
}

func (c *DestroyEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "Do not ask for confirmation")
}

func (c *DestroyEnvironmentCommand) Run(ctx *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}

	if !c.assumeYes {
		var answer string
		fmt.Fprintf(ctx.Stdout, destroyEnvMsg[1:], environ.Name(), environ.Config().Type())
		_, err = fmt.Fscanln(ctx.Stdin, &answer)
		if err != nil {
			return err
		}
		answer = strings.ToLower(answer)
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(ctx.Stdout, "Environment destruction aborted")
			return nil
		}
	}

	return environ.Destroy(nil)
}

const destroyEnvMsg = `
WARNING: this command will destroy the %q environment (type: %s)
This includes all machines, services, data and other resources.

Continue [y/N]? `
