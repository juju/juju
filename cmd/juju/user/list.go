// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/usermanager"
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
	return modelcmd.WrapController(&listCommand{})
}

// listCommand shows all the users in the Juju server.
type listCommand struct {
	infoCommandBase
	All bool
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

// Run implements Command.Run.
func (c *listCommand) Run(ctx *cmd.Context) (err error) {
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
	users, valueConverted := value.([]UserInfo)
	if !valueConverted {
		return errors.Errorf("expected value of type %T, got %T", users, value)
	}
	accountDetails, err := c.ClientStore().AccountDetails(c.ControllerName())
	if err != nil {
		return err
	}
	loggedInUser := names.NewUserTag(accountDetails.User).Canonical()

	tw := output.TabWriter(writer)
	fmt.Fprintf(tw, "CONTROLLER: %v\n", c.ControllerName())
	fmt.Fprint(tw, "\n")
	fmt.Fprint(tw, "NAME\tDISPLAY NAME\tACCESS\tDATE CREATED\tLAST CONNECTION\n")
	for _, user := range users {
		conn := user.LastConnection
		if user.Disabled {
			conn += " (disabled)"
		}
		userName := user.Username
		if loggedInUser == names.NewUserTag(user.Username).Canonical() {
			userName += "*"
			output.CurrentHighlight.Fprintf(tw, "%s\t", userName)
		} else {
			fmt.Fprintf(tw, "%s\t", userName)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", user.DisplayName, user.Access, user.DateCreated, conn)
	}
	tw.Flush()
	return nil
}
