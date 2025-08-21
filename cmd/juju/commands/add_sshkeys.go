// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"errors"
	"fmt"

	"github.com/juju/cmd/v3"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageAddSSHKeySummary = `
Adds a public SSH key to a model.`[1:]

var usageAddSSHKeyDetails = `
Juju maintains a per-model cache of public SSH keys which it copies to
each unit (including units already deployed). By default this includes the
key of the user who created the model (assuming it is stored in the
default location ` + "`~/.ssh/`" + `). Additional keys may be added with this command,
quoting the entire public key as an argument.

`[1:]

const usageAddSSHKeyExamples = `
    juju add-ssh-key "ssh-rsa qYfS5LieM79HIOr535ret6xy
    AAAAB3NzaC1yc2EAAAADAQA6fgBAAABAQCygc6Rc9XgHdhQqTJ
    Wsoj+I3xGrOtk21xYtKijnhkGqItAHmrE5+VH6PY1rVIUXhpTg
    pSkJsHLmhE29OhIpt6yr8vQSOChqYfS5LieM79HIOJEgJEzIqC
    52rCYXLvr/BVkd6yr4IoM1vpb/n6u9o8v1a0VUGfc/J6tQAcPR
    ExzjZUVsfjj8HdLtcFq4JLYC41miiJtHw4b3qYu7qm3vh4eCiK
    1LqLncXnBCJfjj0pADXaL5OQ9dmD3aCbi8KFyOEs3UumPosgmh
    VCAfjjHObWHwNQ/ZU2KrX1/lv/+lBChx2tJliqQpyYMiA3nrtS
    jfqQgZfjVF5vz8LESQbGc6+vLcXZ9KQpuYDt joe@ubuntu"

For ease of use it is possible to use shell substitution to pass the key
to the command:

    juju add-ssh-key "$(cat ~/mykey.pub)"

`

// NewAddKeysCommand is used to add a new ssh key to a model.
func NewAddKeysCommand() cmd.Command {
	return modelcmd.Wrap(&addKeysCommand{})
}

// addKeysCommand is used to add a new authorized ssh key for a user.
type addKeysCommand struct {
	SSHKeysBase
	user    string
	sshKeys []string
}

// Info implements Command.Info.
func (c *addKeysCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-ssh-key",
		Args:     "<ssh key> ...",
		Purpose:  usageAddSSHKeySummary,
		Doc:      usageAddSSHKeyDetails,
		Examples: usageAddSSHKeyExamples,
		SeeAlso: []string{
			"ssh-keys",
			"remove-ssh-key",
			"import-ssh-key",
		},
	})
}

// Init implements Command.Init.
func (c *addKeysCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no ssh key specified")
	default:
		c.sshKeys = args
	}
	return nil
}

// Run implements Command.Run.
func (c *addKeysCommand) Run(context *cmd.Context) error {
	client, err := c.NewKeyManagerClient()
	if err != nil {
		return err
	}
	defer client.Close()
	// TODO(alexisb) - currently keys are global which is not ideal.
	// keymanager needs to be updated to allow keys per user
	c.user = "admin"
	results, err := client.AddKeys(c.user, c.sshKeys...)
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	for i, result := range results {
		if result.Error != nil {
			fmt.Fprintf(context.Stderr, "cannot add key %q: %v\n", c.sshKeys[i], result.Error)
		}
	}
	return nil
}
