// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshkeys

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4/ssh"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageListSSHKeysSummary = `
Lists your authorized SSH keys in a model.`[1:]

var usageListSSHKeysDetails = (`
The command prints the model name followed by one fingerprint per line.
To print public keys instead of fingerprints, use ` + "`--full`" + `.
If the list is empty, the command reports that there are no keys to display.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/ssh-key/

`)[1:]

const usageListSSHKeysExamples = `
List the fingerprints of your registered SSH keys in the current model:

    juju ssh-keys

List your full public SSH keys in the ` + "`jujutest`" + ` model:

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
func (c *listKeysCommand) Run(ctx *cmd.Context) error {
	client, err := c.NewKeyManagerClient(ctx)
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
	results, err := client.ListKeys(ctx, mode, c.user)
	if err != nil {
		return errors.Trace(err)
	}
	result := results[0]
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	if len(result.Result) == 0 {
		ctx.Infof("No keys to display.")
		return nil
	}
	modelIdentifier, err := c.ModelIdentifier()
	if err != nil {
		return errors.Trace(err)
	}
	_, _ = fmt.Fprintf(ctx.Stdout, "Keys used in model: %s\n", modelIdentifier)
	_, _ = fmt.Fprintln(ctx.Stdout, strings.Join(result.Result, "\n"))
	return nil
}
