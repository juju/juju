// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api"
)

var NoEnvironmentError = errors.New("no environment specified")

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	cmd.CommandBase
	envName   string
	assumeYes bool
}

func (c *DestroyEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-environment",
		Args:    "<environment name>",
		Purpose: "terminate all machines and other associated resources for an environment",
	}
}

func (c *DestroyEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not ask for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

func (c *DestroyEnvironmentCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	environ, err := environs.NewFromName(c.envName, store)
	if err != nil {
		return err
	}
	if !c.assumeYes {
		fmt.Fprintf(ctx.Stdout, destroyEnvMsg, environ.Name(), environ.Config().Type())

		scanner := bufio.NewScanner(ctx.Stdin)
		scanner.Scan()
		err := scanner.Err()
		if err != nil && err != io.EOF {
			return fmt.Errorf("Environment destruction aborted: %s", err)
		}
		answer := strings.ToLower(scanner.Text())
		if answer != "y" && answer != "yes" {
			return errors.New("Environment destruction aborted")
		}
	}

	// First, cleanly remove Juju from the environment.
	conn, err := juju.NewAPIConn(environ, api.DefaultDialOpts())
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.State.Client().DestroyJuju(); err != nil {
		return fmt.Errorf("could not remove agents: %v", err)
	}
	// Finally, allow the provider to release environment resources.
	return environs.Destroy(environ, store)
}

func (c *DestroyEnvironmentCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return NoEnvironmentError
	case 1:
		c.envName = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

var destroyEnvMsg = `
WARNING! this command will destroy the %q environment (type: %s)
This includes all machines, services, data and other resources.

Continue [y/N]? `[1:]
