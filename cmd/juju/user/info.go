// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/usermanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/rpc/params"
)

var helpSummary = `
Show information about a user.`[1:]

var helpDetails = `
By default, the ` + "`YAML`" + ` format is used and the user name is the current
user.
`[1:]

const helpExamples = `
    juju show-user
    juju show-user jsmith
    juju show-user --format json
    juju show-user --format yaml
`

// UserInfoAPI defines the API methods that the info command uses.
type UserInfoAPI interface {
	UserInfo([]string, usermanager.IncludeDisabled) ([]params.UserInfo, error)
	Close() error
}

// infoCommandBase is a common base for 'juju show-user' and 'juju users'.
type infoCommandBase struct {
	modelcmd.ControllerCommandBase
	api       UserInfoAPI
	clock     clock.Clock
	exactTime bool
	out       cmd.Output
}

func (c *infoCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.exactTime, "exact-time", false, "Use full timestamp for connection times")
}

func NewShowUserCommand() cmd.Command {
	return modelcmd.WrapController(&infoCommand{
		infoCommandBase: infoCommandBase{
			clock: clock.WallClock,
		},
	})
}

// infoCommand retrieves information about a single user.
type infoCommand struct {
	infoCommandBase
	Username string
}

// UserInfo defines the serialization behaviour of the user information.
type UserInfo struct {
	Username       string `yaml:"user-name" json:"user-name"`
	DisplayName    string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access         string `yaml:"access" json:"access"`
	DateCreated    string `yaml:"date-created,omitempty" json:"date-created,omitempty"`
	LastConnection string `yaml:"last-connection,omitempty" json:"last-connection,omitempty"`
	Disabled       bool   `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Info implements Command.Info.
func (c *infoCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-user",
		Args:     "[<user name>]",
		Purpose:  helpSummary,
		Doc:      helpDetails,
		Examples: helpExamples,
		SeeAlso: []string{
			"add-user",
			"register",
			"users",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *infoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.infoCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
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
		accountDetails, err := c.CurrentAccountDetails()
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
	var now = c.clock.Now()
	for _, info := range users {
		outInfo := UserInfo{
			Username:    info.Username,
			DisplayName: info.DisplayName,
			Access:      info.Access,
			Disabled:    info.Disabled,
		}
		// TODO(wallyworld) record login information about external users.
		if names.NewUserTag(info.Username).IsLocal() {
			outInfo.LastConnection = common.LastConnection(info.LastConnection, now, c.exactTime)
			if c.exactTime {
				outInfo.DateCreated = info.DateCreated.String()
			} else {
				outInfo.DateCreated = common.UserFriendlyDuration(info.DateCreated, now)
			}
		}
		output = append(output, outInfo)
	}

	return output
}
