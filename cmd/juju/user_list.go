// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package main

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const userListCommandDoc = `
List all the current users in the Juju server.

See Also:
   juju user info
`

type UserListCommand struct {
	UserInfoCommandBase
	disabled bool
}

func (c *UserListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "shows all users",
		Doc:     userListCommandDoc,
	}
}

func (c *UserListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.UserInfoCommandBase.SetFlags(f)
	f.BoolVar(&c.disabled, "show-disabled", false, "include disabled users in the listing")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

func (c *UserListCommand) Run(ctx *cmd.Context) (err error) {
	// Note: the getUserInfoAPI and the UserInfo struct are defined
	// in user_info.go.
	client, err := getUserInfoAPI(&c.UserCommandBase)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.UserInfo(nil, c.disabled)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, c.apiUsersToUserInfoSlice(result))
}

func (c *UserListCommand) formatTabular(value interface{}) ([]byte, error) {
	users, valueConverted := value.([]UserInfo)
	if !valueConverted {
		return nil, errors.Errorf("expected value of type %T, got %T", users, value)
	}
	var out bytes.Buffer
	// To format things into columns.
	minwidth := 0
	tabwidth := 1
	padding := 2
	padchar := byte(' ')
	flags := uint(0)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME\tDISPLAY NAME\tDATE CREATED\tLAST CONNECTION\n")
	for _, user := range users {
		conn := user.LastConnection
		if user.Disabled {
			conn += " (disabled)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", user.Username, user.DisplayName, user.DateCreated, conn)
	}
	tw.Flush()
	return out.Bytes(), nil
}
