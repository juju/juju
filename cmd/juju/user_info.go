// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const userInfoCommandDoc = `
Display infomation on a user.

Examples:
  	# Show information on the current user
  	$ juju user info  
  	user-name: foobar
  	display-name: Foo Bar
  	date-created : 1981-02-27 16:10:05 +0000 UTC
	last-connection: 2014-01-01 00:00:00 +0000 UTC

  	# Show information on a user with the given username
  	$ juju user info jsmith
  	user-name: jsmith
  	display-name: John Smith
  	date-created : 1981-02-27 16:10:05 +0000 UTC
	last-connection: 2014-01-01 00:00:00 +0000 UTC

  	# Show information on the current user in JSON format
  	$ juju user info --format json
  	{"user-name":"foobar",
  	"display-name":"Foo Bar",
	"date-created": "1981-02-27 16:10:05 +0000 UTC",
	"last-connection": "2014-01-01 00:00:00 +0000 UTC"}

  	# Show information on the current user in YAML format
  	$ juju user info --format yaml
 	user-name: foobar
 	display-name: Foo Bar
 	date-created : 1981-02-27 16:10:05 +0000 UTC
	last-connection: 2014-01-01 00:00:00 +0000 UTC
`

type UserInfoCommand struct {
	UserCommandBase
	Username string
	out      cmd.Output
}

type UserInfo struct {
	Username       string `yaml:"user-name" json:"user-name"`
	DisplayName    string `yaml:"display-name" json:"display-name"`
	DateCreated    string `yaml:"date-created" json:"date-created"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
	Disabled       bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

func (c *UserInfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "info",
		Args:    "<username>",
		Purpose: "shows information on a user",
		Doc:     userInfoCommandDoc,
	}
}

func (c *UserInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

func (c *UserInfoCommand) Init(args []string) (err error) {
	c.Username, err = cmd.ZeroOrOneArgs(args)
	return err
}

type UserInfoAPI interface {
	UserInfo(tags []names.UserTag, includeDeactivated bool) ([]params.UserInfo, error)
	Close() error
}

var getUserInfoAPI = func(c *UserInfoCommand) (UserInfoAPI, error) {
	return c.NewUserManagerClient()
}

func (c *UserInfoCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getUserInfoAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	username := c.Username
	if username == "" {
		info, err := c.ConnectionCredentials()
		if err != nil {
			return err
		}
		username = info.User
	}
	userTag := names.NewLocalUserTag(username)
	result, err := client.UserInfo([]names.UserTag{userTag}, false)
	if err != nil {
		return err
	}
	if len(result) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(result))
	}
	// Don't output the params type, but be explicit
	info := result[0]
	outInfo := UserInfo{
		Username:    info.Username,
		DisplayName: info.DisplayName,
		DateCreated: info.DateCreated.String(),
		Disabled:    info.Deactivated,
	}
	if info.LastConnection != nil {
		outInfo.LastConnection = info.LastConnection.String()
	} else {
		outInfo.LastConnection = "not connected yet"
	}
	if err = c.out.Write(ctx, outInfo); err != nil {
		return err
	}
	return nil
}
