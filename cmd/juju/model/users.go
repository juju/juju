// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/user"
	"github.com/juju/juju/cmd/modelcmd"
)

const userCommandDoc = `List all users with access to the current model`

func NewUsersCommand() cmd.Command {
	return modelcmd.Wrap(&usersCommand{})
}

// usersCommand shows all the users with access to the current model.
type usersCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output
	api UsersAPI
}

// UserInfo defines the serialization behaviour of the user information.
type UserInfo struct {
	DisplayName    string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access         string `yaml:"access" json:"access"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
}

// UsersAPI defines the methods on the client API that the
// users command calls.
type UsersAPI interface {
	Close() error
	ModelUserInfo() ([]params.ModelUserInfo, error)
}

func (c *usersCommand) getAPI() (UsersAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// Info implements Command.Info.
func (c *usersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-shares",
		Purpose: "shows all users with access to the current model",
		Doc:     userCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *usersCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Run implements Command.Run.
func (c *usersCommand) Run(ctx *cmd.Context) (err error) {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.ModelUserInfo()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, apiUsersToUserInfoMap(result))
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *usersCommand) formatTabular(value interface{}) ([]byte, error) {
	users, ok := value.(map[string]UserInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", users, value)
	}
	var out bytes.Buffer
	if err := formatTabularUserInfo(users, &out); err != nil {
		return nil, errors.Trace(err)
	}
	return out.Bytes(), nil
}

func formatTabularUserInfo(users map[string]UserInfo, out *bytes.Buffer) error {
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	names := make(set.Strings)
	for name := range users {
		names.Add(name)
	}
	tw := tabwriter.NewWriter(out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME\tACCESS\tLAST CONNECTION\n")
	for _, name := range names.SortedValues() {
		user := users[name]
		displayName := name
		if user.DisplayName != "" {
			displayName = fmt.Sprintf("%s (%s)", name, user.DisplayName)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", displayName, user.Access, user.LastConnection)
	}
	tw.Flush()
	return nil
}

func apiUsersToUserInfoMap(users []params.ModelUserInfo) map[string]UserInfo {
	output := make(map[string]UserInfo)
	for _, info := range users {
		outInfo := UserInfo{
			DisplayName: info.DisplayName,
			Access:      string(info.Access),
		}
		if info.LastConnection != nil {
			outInfo.LastConnection = user.UserFriendlyDuration(*info.LastConnection, time.Now())
		} else {
			outInfo.LastConnection = "never connected"
		}
		output[info.UserName] = outInfo
	}
	return output
}
