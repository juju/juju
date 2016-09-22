// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

var usageListUsersSummary = `
Lists Juju users allowed to connect to a controller.`[1:]

var usageListUsersDetails = `
By default, the tabular format is used.

Examples:
    juju users

See also: 
    add-user
    register
    show-user
    disable-user
    enable-user`[1:]

func NewListCommand() cmd.Command {
	return modelcmd.WrapController(&listCommand{
		infoCommandBase: infoCommandBase{
			clock: clock.WallClock,
		},
	})
}

// listCommand shows all the users in the Juju server.
type listCommand struct {
	infoCommandBase
	modelUserAPI modelUsersAPI

	All         bool
	modelName   string
	currentUser string
}

// ModelUsersAPI defines the methods on the client API that the
// users command calls.
type modelUsersAPI interface {
	Close() error
	ModelUserInfo() ([]params.ModelUserInfo, error)
}

func (c *listCommand) getModelAPI() (modelUsersAPI, error) {
	if c.modelUserAPI != nil {
		return c.modelUserAPI, nil
	}
	conn, err := c.NewModelAPIRoot(c.modelName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn.Client(), nil
}

// Info implements Command.Info.
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "users",
		Purpose: usageListUsersSummary,
		Doc:     usageListUsersDetails,
		Aliases: []string{"list-users"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.infoCommandBase.SetFlags(f)
	f.BoolVar(&c.All, "all", false, "Include disabled users")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Init implements Command.Init.
func (c *listCommand) Init(args []string) (err error) {
	c.modelName, err = cmd.ZeroOrOneArgs(args)
	if err != nil {
		return err
	}
	return err
}

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
	if c.out.Name() == "tabular" {
		// Only the tabular outputters need to know the current user,
		// but both of them do, so do it in one place.
		accountDetails, err := c.ClientStore().AccountDetails(c.ControllerName())
		if err != nil {
			return err
		}
		c.currentUser = names.NewUserTag(accountDetails.User).Canonical()
	}
	if c.modelName == "" {
		return c.controllerUsers(ctx)
	}
	return c.modelUsers(ctx)
}

func (c *listCommand) modelUsers(ctx *cmd.Context) error {
	client, err := c.getModelAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ModelUserInfo()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, common.ModelUserInfoFromParams(result, c.clock.Now()))
}

func (c *listCommand) controllerUsers(ctx *cmd.Context) error {
	// Note: the InfoCommandBase and the UserInfo struct are defined
	// in info.go.
	client, err := c.getUserInfoAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.UserInfo(nil, usermanager.IncludeDisabled(c.All))
	if err != nil {
		return err
	}

	return c.out.Write(ctx, c.apiUsersToUserInfoSlice(result))
}

func (c *listCommand) formatTabular(writer io.Writer, value interface{}) error {
	if c.modelName == "" {
		return c.formatControllerUsers(writer, value)
	}
	return c.formatModelUsers(writer, value)
}

func (c *listCommand) isLoggedInUser(username string) bool {
	tag := names.NewUserTag(username)
	return tag.Canonical() == c.currentUser
}

func (c *listCommand) formatModelUsers(writer io.Writer, value interface{}) error {
	users, ok := value.(map[string]common.ModelUserInfo)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", users, value)
	}
	modelUsers := set.NewStrings()
	for name := range users {
		modelUsers.Add(name)
	}
	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println("NAME", "DISPLAY NAME", "ACCESS", "LAST CONNECTION")
	for _, name := range modelUsers.SortedValues() {
		user := users[name]

		var highlight *ansiterm.Context
		userName := name
		if c.isLoggedInUser(name) {
			userName += "*"
			highlight = output.CurrentHighlight
		}
		w.PrintColor(highlight, userName)
		w.Println(user.DisplayName, user.Access, user.LastConnection)
	}
	tw.Flush()
	return nil
}

func (c *listCommand) formatControllerUsers(writer io.Writer, value interface{}) error {
	users, valueConverted := value.([]UserInfo)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", users, value)
	}

	tw := output.TabWriter(writer)
	w := output.Wrapper{tw}
	w.Println("CONTROLLER: " + c.ControllerName())
	w.Println()
	w.Println("NAME", "DISPLAY NAME", "ACCESS", "DATE CREATED", "LAST CONNECTION")
	for _, user := range users {
		conn := user.LastConnection
		if user.Disabled {
			conn += " (disabled)"
		}
		var highlight *ansiterm.Context
		userName := user.Username
		if c.isLoggedInUser(user.Username) {
			userName += "*"
			highlight = output.CurrentHighlight
		}
		w.PrintColor(highlight, userName)
		w.Println(user.DisplayName, user.Access, user.DateCreated, conn)
	}
	tw.Flush()
	return nil
}
