// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"
)

const disableUserDoc = `
Disabling a user stops that user from being able to log in. The user still
exists and can be reenabled using the "--enable" flag.

Examples:
  juju user disable foobar

  juju user disable --enable foobar
`

type DisableUserCommand struct {
	UserCommandBase
	user   string
	enable bool
}

func (c *DisableUserCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "disable",
		Args:    "<username>",
		Purpose: "disable a user to disallow login",
		Doc:     disableUserDoc,
	}
}

func (c *DisableUserCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.enable, "enable", false, "re-enable a disabled user")
}

func (c *DisableUserCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no username supplied")
	}
	c.user = args[0]
	if !names.IsValidUser(c.user) {
		return errors.Errorf("%q is not a valid username", c.user)
	}
	return cmd.CheckEmpty(args[1:])
}

type disableUserAPI interface {
	ActivateUser(tag names.UserTag) error
	DeactivateUser(tag names.UserTag) error
	Close() error
}

var getDisableUserAPI = func(c *DisableUserCommand) (disableUserAPI, error) {
	return c.NewUserManagerClient()
}

func (c *DisableUserCommand) Run(_ *cmd.Context) error {
	client, err := getDisableUserAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()

	tag := names.NewUserTag(c.user)
	if c.enable {
		return client.ActivateUser(tag)
	}
	return client.DeactivateUser(tag)
}
