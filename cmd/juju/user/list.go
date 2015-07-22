// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package user

import (
	"bytes"
	"fmt"
	"text/tabwriter"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api/usermanager"
)

const ListCommandDoc = `
List all the current users in the Juju server.

See Also:
   juju help user info
`

// ListCommand shows all the users in the Juju server.
type ListCommand struct {
	InfoCommandBase
	all bool
}

// Info implements Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "shows all users",
		Doc:     ListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.InfoCommandBase.SetFlags(f)
	f.BoolVar(&c.all, "all", false, "include disabled users in the listing")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatTabular,
	})
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) (err error) {
	// Note: the InfoCommandBase and the UserInfo struct are defined
	// in info.go.
	client, err := c.getUserInfoAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.UserInfo(nil, usermanager.IncludeDisabled(c.all))
	if err != nil {
		return err
	}

	return c.out.Write(ctx, c.apiUsersToUserInfoSlice(result))
}

func (c *ListCommand) formatTabular(value interface{}) ([]byte, error) {
	users, valueConverted := value.([]UserInfo)
	if !valueConverted {
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
