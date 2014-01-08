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
	"launchpad.net/juju-core/state/api/params"
)

var NoEnvironmentError = errors.New("no environment specified")

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	cmd.CommandBase
	envName   string
	assumeYes bool
	force     bool
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
	f.BoolVar(&c.force, "force", false, "Forcefully destroy the environment, directly through the environment provider")
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
	// If --force is supplied, then don't attempt to use the API.
	// This is necessary to destroy broken environments, where the
	// API server is inaccessible or faulty.
	if !c.force {
		conn, err := juju.NewAPIConn(environ, api.DefaultDialOpts())
		if err != nil {
			return err
		}
		defer conn.Close()
		err = conn.State.Client().DestroyEnvironment()
		if err != nil && !params.IsCodeNotImplemented(err) {
			return fmt.Errorf("destroying environment: %v", err)
		}
	}
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
