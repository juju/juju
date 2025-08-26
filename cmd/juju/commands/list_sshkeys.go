// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3/ssh"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageListSSHKeysSummary = `
Lists the currently known SSH keys for the current (or specified) model.`[1:]

var usageListSSHKeysDetails = `
Juju maintains a per-model cache of SSH keys which it copies to each newly
created unit.

This command will display a list of all the keys currently used by Juju in
the current model (or the model specified, if the ` + "`-m`" + ` option is used).

By default a minimal list is returned, showing only the fingerprint of
each key and its text identifier. By using the ` + "`--full`" + ` option, the entire
key may be displayed.

`[1:]

const usageListSSHKeysExamples = `
    juju ssh-keys

To examine the full information for each key:

    juju ssh-keys -m jujutest --full
`

// NewListKeysCommand returns a command used to list the authorized ssh keys.
func NewListKeysCommand() cmd.Command {
	return modelcmd.Wrap(&listKeysCommand{})
}

// listKeysCommand is used to list the authorized ssh keys.
type listKeysCommand struct {
	SSHKeysBase
	showFullKey bool
	user        string
}

// Info implements Command.Info.
func (c *listKeysCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "ssh-keys",
		Purpose:  usageListSSHKeysSummary,
		Doc:      usageListSSHKeysDetails,
		Aliases:  []string{"list-ssh-keys"},
		Examples: usageListSSHKeysExamples,
		SeeAlso: []string{
			"add-ssh-key",
			"remove-ssh-key",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *listKeysCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SSHKeysBase.SetFlags(f)
	f.BoolVar(&c.showFullKey, "full", false, "Show full key instead of just the fingerprint")
}

// Run implements Command.Run.
func (c *listKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	mode := ssh.Fingerprints
	if c.showFullKey {
		mode = ssh.FullKeys
	}
	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.ListKeys(mode, c.user)
	if err != nil {
		return errors.Trace(err)
	}
	result := results[0]
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	if len(result.Result) == 0 {
		context.Infof("No keys to display.")
		return nil
	}
	modelIdentifier, err := c.ModelIdentifier()
	if err != nil {
		return errors.Trace(err)
	}
	_, _ = fmt.Fprintf(context.Stdout, "Keys used in model: %s\n", modelIdentifier)
	_, _ = fmt.Fprintln(context.Stdout, strings.Join(result.Result, "\n"))
	return nil
}
