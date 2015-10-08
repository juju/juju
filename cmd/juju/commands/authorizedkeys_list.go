// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/utils/ssh"
)

func newListKeysCommand() cmd.Command {
	return envcmd.Wrap(&listKeysCommand{})
}

var listKeysDoc = `
List a user's authorized ssh keys, allowing the holders of those keys to log on to Juju nodes.
By default, just the key fingerprint is printed. Use --full to display the entire key.

`

// listKeysCommand is used to list the authorized ssh keys.
type listKeysCommand struct {
	AuthorizedKeysBase
	showFullKey bool
	user        string
}

func (c *listKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Doc:     listKeysDoc,
		Purpose: "list authorised ssh keys for a specified user",
	}
}

func (c *listKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.showFullKey, "full", false, "show full key instead of just the key fingerprint")
	f.StringVar(&c.user, "user", "admin", "the user for which to list the keys")
}

func (c *listKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()

	mode := ssh.Fingerprints
	if c.showFullKey {
		mode = ssh.FullKeys
	}
	results, err := client.ListKeys(mode, c.user)
	if err != nil {
		return err
	}
	result := results[0]
	if result.Error != nil {
		return result.Error
	}
	fmt.Fprintf(context.Stdout, "Keys for user %s:\n", c.user)
	fmt.Fprintln(context.Stdout, strings.Join(result.Result, "\n"))
	return nil
}
