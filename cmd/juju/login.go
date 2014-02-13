// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"

	"code.google.com/p/gopass"
)

type LoginCommand struct {
	cmd.EnvCommandBase
	Tag      string
	Password string
}

func (c *LoginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "login",
		Args:    "<username> <password>",
		Purpose: "login as user",
	}
}

func (c *LoginCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	c.Tag = args[0]

	if len(args) == 1 {
		pass, err := gopass.GetPass("password: ")
		if err != nil {
			return fmt.Errorf("Failed to read password %v", err)
		}
		c.Password = pass
	} else {
		c.Password = args[1]
	}
	return nil
}

func (c *LoginCommand) Run(_ *cmd.Context) error {
	client, err := juju.NewAPIClientFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer client.Close()
	return nil
}
