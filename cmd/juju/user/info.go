// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
)

const InfoCommandDoc = `
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

// InfoCommand retrieves information about a single user.
type InfoCommand struct {
	UserCommandBase
	exactTime bool
	out       cmd.Output
}

type UserInfoCommand struct {
	UserInfoCommandBase
	Username string
}

// UserInfo defines the serialization behaviour of the user information.
type UserInfo struct {
	Username       string `yaml:"user-name" json:"user-name"`
	DisplayName    string `yaml:"display-name" json:"display-name"`
	DateCreated    string `yaml:"date-created" json:"date-created"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
	Disabled       bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Info implements Command.Info.
func (c *InfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "info",
		Args:    "<username>",
		Purpose: "shows information on a user",
		Doc:     InfoCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *InfoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

// Init implements Command.Init.
func (c *InfoCommand) Init(args []string) (err error) {
	c.Username, err = cmd.ZeroOrOneArgs(args)
	return err
}

// UserInfoAPI defines the API methods that the info command uses.
type UserInfoAPI interface {
	UserInfo([]string, usermanager.IncludeDisabled) ([]params.UserInfo, error)
	Close() error
}

func (c *InfoCommand) getUserInfoAPI() (UserInfoAPI, error) {
	return c.NewUserManagerClient()
}

var getUserInfoAPI = (*InfoCommand).getUserInfoAPI

// Run implements Command.Run.
func (c *InfoCommand) Run(ctx *cmd.Context) (err error) {
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
	result, err := client.UserInfo([]string{username}, false)
	if err != nil {
		return err
	}
	if len(result) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(result))
	}
	// Don't output the params type, be explicit.
	info := result[0]
	outInfo := UserInfo{
		Username:    info.Username,
		DisplayName: info.DisplayName,
		DateCreated: info.DateCreated.String(),
		Disabled:    info.Disabled,
	}
	if info.LastConnection != nil {
		outInfo.LastConnection = info.LastConnection.String()
	} else {
		outInfo.LastConnection = "never connected"
	}
	if err = c.out.Write(ctx, outInfo); err != nil {
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
