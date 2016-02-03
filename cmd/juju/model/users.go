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
	Username       string `yaml:"user-name" json:"user-name"`
	DateCreated    string `yaml:"date-created" json:"date-created"`
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

	return c.out.Write(ctx, c.apiUsersToUserInfoSlice(result))
}

// formatTabular takes an interface{} to adhere to the cmd.Formatter interface
func (c *usersCommand) formatTabular(value interface{}) ([]byte, error) {
	users, ok := value.([]UserInfo)
	if !ok {
		return nil, errors.Errorf("expected value of type %T, got %T", users, value)
	}
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME\tDATE CREATED\tLAST CONNECTION\n")
	for _, user := range users {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", user.Username, user.DateCreated, user.LastConnection)
	}
	tw.Flush()
	return out.Bytes(), nil
}

func (c *usersCommand) apiUsersToUserInfoSlice(users []params.ModelUserInfo) []UserInfo {
	var output []UserInfo
	for _, info := range users {
		outInfo := UserInfo{Username: info.UserName}
		outInfo.DateCreated = user.UserFriendlyDuration(info.DateCreated, time.Now())
		if info.LastConnection != nil {
			outInfo.LastConnection = user.UserFriendlyDuration(*info.LastConnection, time.Now())
		} else {
			outInfo.LastConnection = "never connected"
		}

		output = append(output, outInfo)
	}

	return output
}
