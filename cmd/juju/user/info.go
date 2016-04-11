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
	"github.com/juju/juju/cmd/modelcmd"
)

var helpSummary = `
Show information about a user.`[1:]

var helpDetails = `
By default, the YAML format is used and the user name is the current user.


Examples:
    juju show-user
    juju show-user jsmith
    juju show-user --format json
    juju show-user --format yaml
    
See also: 
    add-user
    register
    list-users`[1:]

// UserInfoAPI defines the API methods that the info command uses.
type UserInfoAPI interface {
	UserInfo([]string, usermanager.IncludeDisabled) ([]params.UserInfo, error)
	Close() error
}

// infoCommandBase is a common base for 'juju show-user' and 'juju list-user'.
type infoCommandBase struct {
	modelcmd.ControllerCommandBase
	api       UserInfoAPI
	exactTime bool
	out       cmd.Output
}

func (c *infoCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.exactTime, "exact-time", false, "use full timestamp precision")
}

func NewShowUserCommand() cmd.Command {
	return modelcmd.WrapController(&infoCommand{})
}

// infoCommand retrieves information about a single user.
type infoCommand struct {
	infoCommandBase
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
func (c *infoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-user",
		Args:    "[<user name>]",
		Purpose: helpSummary,
		Doc:     helpDetails,
	}
}

// SetFlags implements Command.SetFlags.
func (c *infoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.infoCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

// Init implements Command.Init.
func (c *infoCommand) Init(args []string) (err error) {
	c.Username, err = cmd.ZeroOrOneArgs(args)
	return err
}

func (c *infoCommandBase) getUserInfoAPI() (UserInfoAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewUserManagerAPIClient()
}

// Run implements Command.Run.
func (c *infoCommand) Run(ctx *cmd.Context) (err error) {
	client, err := c.getUserInfoAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	username := c.Username
	if username == "" {
		accountDetails, err := c.ClientStore().AccountByName(
			c.ControllerName(), c.AccountName(),
		)
		if err != nil {
			return err
		}
		username = accountDetails.User
	}
	result, err := client.UserInfo([]string{username}, false)
	if err != nil {
		return err
	}
	// Don't output the params type, be explicit. We convert before checking
	// length because we want to reuse the conversion function, and we are
	// pretty sure that there is one value there.
	output := c.apiUsersToUserInfoSlice(result)
	if len(output) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(output))
	}
	return c.out.Write(ctx, output[0])
}

func (c *infoCommandBase) apiUsersToUserInfoSlice(users []params.UserInfo) []UserInfo {
	var output []UserInfo
	var now = time.Now()
	for _, info := range users {
		outInfo := UserInfo{
			Username:       info.Username,
			DisplayName:    info.DisplayName,
			Disabled:       info.Disabled,
			LastConnection: LastConnection(info.LastConnection, now, c.exactTime),
		}
		if c.exactTime {
			outInfo.DateCreated = info.DateCreated.String()
		} else {
			outInfo.DateCreated = UserFriendlyDuration(info.DateCreated, now)
		}

		output = append(output, outInfo)
	}

	return output
}

// LastConnection turns the *time.Time returned from the API server
// into a user facing string with either exact time or a user friendly
// string based on the args.
func LastConnection(connectionTime *time.Time, now time.Time, exact bool) string {
	if connectionTime == nil {
		return "never connected"
	}
	if exact {
		return connectionTime.String()
	}
	return UserFriendlyDuration(*connectionTime, now)
}

// UserFriendlyDuration translates a time in the past into a user
// friendly string representation relative to the "now" time argument.
func UserFriendlyDuration(when, now time.Time) string {
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
