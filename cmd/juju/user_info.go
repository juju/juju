// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package main

import (
	"fmt"
	"time"

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

type UserInfoCommandBase struct {
	UserCommandBase
	exactTime bool
	out       cmd.Output
}

type UserInfoCommand struct {
	UserInfoCommandBase
	Username string
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

func (c *UserInfoCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.exactTime, "exact-time", false, "use full timestamp precision")
}

func (c *UserInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UserInfoCommandBase.SetFlags(f)
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

var getUserInfoAPI = func(c *UserCommandBase) (UserInfoAPI, error) {
	return c.NewUserManagerClient()
}

func (c *UserInfoCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getUserInfoAPI(&c.UserCommandBase)
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
	// Don't output the params type, but be explicit. We convert before
	// checking length because we want to reuse the conversion function, and
	// we are pretty sure that there is one value there.
	output := c.apiUsersToUserInfoSlice(result)
	if len(output) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(output))
	}
	if err = c.out.Write(ctx, output[0]); err != nil {
		return err
	}
	return nil
}

func (c *UserInfoCommandBase) apiUsersToUserInfoSlice(users []params.UserInfo) []UserInfo {
	var output []UserInfo
	var now = time.Now()
	for _, info := range users {
		outInfo := UserInfo{
			Username:    info.Username,
			DisplayName: info.DisplayName,
			Disabled:    info.Deactivated,
		}
		if c.exactTime {
			outInfo.DateCreated = info.DateCreated.String()
		} else {
			outInfo.DateCreated = userFriendlyDuration(info.DateCreated, now)
		}
		if info.LastConnection != nil {
			if c.exactTime {
				outInfo.LastConnection = info.LastConnection.String()
			} else {
				outInfo.LastConnection = userFriendlyDuration(*info.LastConnection, now)
			}
		} else {
			outInfo.LastConnection = "not connected yet"
		}

		output = append(output, outInfo)
	}

	return output
}

func userFriendlyDuration(when, now time.Time) string {
	since := now.Sub(when)
	// if over 24 hours ago, just say the date.
	if since.Hours() >= 24 {
		return when.Format("2006-01-02")
	}
	if since.Hours() >= 1 {
		unit := "hours"
		if int(since.Hours()) == 1 {
			unit = "hour"
		}
		return fmt.Sprintf("%d %s ago", int(since.Hours()), unit)
	}
	if since.Minutes() >= 1 {
		unit := "minutes"
		if int(since.Minutes()) == 1 {
			unit = "minute"
		}
		return fmt.Sprintf("%d %s ago", int(since.Minutes()), unit)
	}
	if since.Seconds() >= 2 {
		return fmt.Sprintf("%d seconds ago", int(since.Seconds()))
	}
	return "just now"
}
