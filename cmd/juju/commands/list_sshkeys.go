// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/utils/ssh"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewListKeysCommand returns a command used to list the authorized ssh keys.
func NewListKeysCommand() cmd.Command {
	return modelcmd.Wrap(&listKeysCommand{})
}

var listKeysDoc = `
Juju maintains a per-model cache of SSH keys which it copies to each newly
created unit.
This command will display a list of all the keys currently used by Juju in
the current model (or the model specified, if the '-m' option is used).
By default a minimal list is returned, showing only the fingerprint of
each key and its text identifier. By using the '--full' option, the entire
key may be displayed.

Examples:

    juju list-ssh-keys
    juju list-keys -m jujutest --full
`

// listKeysCommand is used to list the authorized ssh keys.
type listKeysCommand struct {
	SSHKeysBase
	showFullKey bool
	user        string
}

// Info implements Command.Info.
func (c *listKeysCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-ssh-keys",
		Doc:     listKeysDoc,
		Purpose: "Lists the currently known SSH keys for the current (or specified) model.",
		Aliases: []string{"ssh-key", "ssh-keys", "list-ssh-key"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *listKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.showFullKey, "full", false, "Show full key instead of just the fingerprint")
}

// Run implements Command.Run.
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
	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.ListKeys(mode, c.user)
	if err != nil {
		return err
	}
	result := results[0]
	if result.Error != nil {
		return result.Error
	}
	fmt.Fprintf(context.Stdout, "Keys used in model: %s\n", c.ConnectionName())
	fmt.Fprintln(context.Stdout, strings.Join(result.Result, "\n"))
	return nil
}
