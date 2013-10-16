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
)

// DestroyEnvironmentCommand destroys an environment.
type DestroyEnvironmentCommand struct {
	cmd.EnvCommandBase
}

func (c *DestroyEnvironmentCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-environment",
		Purpose: "terminate all machines and other associated resources for an environment",
	}
}

func (c *DestroyEnvironmentCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
}

func (c *DestroyEnvironmentCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	environ, err := environs.NewFromName(c.EnvName, store)
	if err != nil {
		return err
	}

	fmt.Fprintf(ctx.Stdout, destroyEnvMsg, environ.Name(), environ.Config().Type(), environ.Name())

	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err = scanner.Err()
	if err != nil && err != io.EOF {
		return fmt.Errorf("Environment destruction aborted. Error reading input: %s", scanner.Err())
	}

	answer := strings.ToLower(scanner.Text())
	if answer != strings.ToLower("destroy "+environ.Name()) {
		return errors.New("Environment destruction aborted")
	}

	// TODO(axw) 2013-08-30 bug 1218688
	// destroy manually provisioned machines, or otherwise
	// block destroy-environment until all manually provisioned
	// machines have been manually "destroyed".
	return environs.Destroy(environ, store)
}

var destroyEnvMsg = `
WARNING! This command will destroy the %q environment (type: %s)
This includes all machines, services, data and other resources.

Type "destroy %s" to continue: `[1:]
