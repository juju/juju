// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
)

// UnsetCommand sets configuration values of a service back
// to their default.
type UnsetCommand struct {
	cmd.EnvCommandBase
	ServiceName string
	Options     []string
}

const unsetDoc = `
Set one or more configuration options for the specified service to their
default. See also the set commmand to set one or more configuration options for
a specified service.
`

func (c *UnsetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unset",
		Args:    "<service> name ...",
		Purpose: "set service config options back to their default",
		Doc:     unsetDoc,
	}
}

func (c *UnsetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
}

func (c *UnsetCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no service name specified")
	}
	c.ServiceName = args[0]
	c.Options = args[1:]
	if len(c.Options) == 0 {
		return errors.New("no configuration options specified")
	}
	return nil
}

// run1dot16 runs 'juju unset' using a direct DB connection to maintain
// compatibility with an API server running 1.16 or older (when ServiceUnset
// was not available). This fallback can be removed when we no longer maintain
// 1.16 compatibility.
// This was copied directly from the code in UnsetCommand.Run in 1.16
func (c *UnsetCommand) run1dot16() error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	service, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	if len(c.Options) > 0 {
		settings := make(charm.Settings)
		for _, option := range c.Options {
			settings[option] = nil
		}
		return service.UpdateConfigSettings(settings)
	} else {
		return nil
	}
}

// Run resets the configuration of a service.
func (c *UnsetCommand) Run(ctx *cmd.Context) error {
	apiclient, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer apiclient.Close()
	err = apiclient.ServiceUnset(c.ServiceName, c.Options)
	if params.IsCodeNotImplemented(err) {
		logger.Infof("ServiceUnset not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		err = c.run1dot16()
	}
	return err
}
