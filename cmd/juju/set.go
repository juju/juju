// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/api/params"
)

// SetCommand updates the configuration of a service.
type SetCommand struct {
	cmd.EnvCommandBase
	ServiceName     string
	SettingsStrings map[string]string
	SettingsYAML    cmd.FileVar
}

const setDoc = `
Set one or more configuration options for the specified service. See also the
unset command which sets one or more configuration options for a specified
service to their default value. 
`

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set",
		Args:    "<service> name=value ...",
		Purpose: "set service config options",
		Doc:     "Set one or more configuration options for the specified service.",
	}
}

func (c *SetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.Var(&c.SettingsYAML, "config", "path to yaml-formatted service config")
}

func (c *SetCommand) Init(args []string) error {
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no service name specified")
	}
	if c.SettingsYAML.Path != "" && len(args) > 1 {
		return errors.New("cannot specify --config when using key=value arguments")
	}
	c.ServiceName = args[0]
	settings, err := parse(args[1:])
	if err != nil {
		return err
	}
	c.SettingsStrings = settings
	return nil
}

// serviceSet1dot16 does the final ServiceSet step using direct DB access
// compatibility with an API server running 1.16 or older (when ServiceUnset
// was not available). This fallback can be removed when we no longer maintain
// 1.16 compatibility.
// This was copied directly from the code in SetCommand.Run in 1.16
func (c *SetCommand) serviceSet1dot16() error {
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	service, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	ch, _, err := service.Charm()
	if err != nil {
		return err
	}
	// We don't need the multiple logic here, because that should have
	// already been taken care of by the API code (which *was* in 1.16).
	settings, err := ch.Config().ParseSettingsStrings(c.SettingsStrings)
	if err != nil {
		return err
	}
	return service.UpdateConfigSettings(settings)
}

// Run updates the configuration of a service.
func (c *SetCommand) Run(ctx *cmd.Context) error {
	api, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer api.Close()

	if c.SettingsYAML.Path != "" {
		b, err := c.SettingsYAML.Read(ctx)
		if err != nil {
			return err
		}
		return api.ServiceSetYAML(c.ServiceName, string(b))
	} else if len(c.SettingsStrings) == 0 {
		return nil
	}
	err = api.ServiceSet(c.ServiceName, c.SettingsStrings)
	if params.IsCodeNotImplemented(err) {
		logger.Infof("NewServiceSetForClientAPI not supported by the API server, " +
			"falling back to 1.16 compatibility mode (direct DB access)")
		err = c.serviceSet1dot16()
	}
	return err
}

// parse parses the option k=v strings into a map of options to be
// updated in the config. Keys with empty values are returned separately
// and should be removed.
func parse(options []string) (map[string]string, error) {
	kv := make(map[string]string)
	for _, o := range options {
		s := strings.SplitN(o, "=", 2)
		if len(s) != 2 || s[0] == "" {
			return nil, fmt.Errorf("invalid option: %q", o)
		}
		kv[s[0]] = s[1]
	}
	return kv, nil
}
