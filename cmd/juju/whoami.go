// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs/configstore"
	//"launchpad.net/juju-core/names"
)

const whoamiDoc = `
`

type WhoamiCommand struct {
	envcmd.EnvCommandBase
	out cmd.Output
}

func (c *WhoamiCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "whoami",
		Args:    "",
		Purpose: "prints the name of the currently logged in user",
		Doc:     whoamiDoc,
	}
}

func (c *WhoamiCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *WhoamiCommand) Init(args []string) error {
	return c.EnsureEnvNameSet()
}

func (c *WhoamiCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return fmt.Errorf("cannot open environment info storage: %v", err)
	}
	info, err := store.ReadInfo(c.EnvName)
	if err != nil {
		return err
	}
	user := info.APICredentials().User
	//_, id, err := names.ParseTag(user, names.UserTagKind)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, user)
}
