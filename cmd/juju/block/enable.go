// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewEnableCommand returns a new command that eanbles previously disabled
// command sets.
func NewEnableCommand() cmd.Command {
	return modelcmd.Wrap(&enableCommand{
		apiFunc: func(c newAPIRoot) (unblockClientAPI, error) {
			return getBlockAPI(c)
		},
	})
}

// enableCommand removes the block from desired operation.
type enableCommand struct {
	modelcmd.ModelCommandBase
	apiFunc func(newAPIRoot) (unblockClientAPI, error)
	target  string
}

// Init implements Command.
func (c *enableCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing command set (%s)", validTargets)
	}
	c.target, args = args[0], args[1:]
	target, ok := toAPIValue[c.target]
	if !ok {
		return errors.Errorf("bad command set, valid options: %s", validTargets)
	}
	c.target = target
	return cmd.CheckEmpty(args)
}

// Info implementsCommand.
func (c *enableCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "enable-command",
		Args:     "<command set>",
		Purpose:  "Enable commands that had been previously disabled.",
		Doc:      enableDoc,
		Examples: enableExamples,
		SeeAlso: []string{
			"disable-command",
			"disabled-commands",
		},
	})
}

// unblockClientAPI defines the client API methods that unblock command uses.
type unblockClientAPI interface {
	Close() error
	SwitchBlockOff(blockType string) error
}

// Run implements Command.
func (c *enableCommand) Run(_ *cmd.Context) error {
	api, err := c.apiFunc(c)
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API")
	}
	defer api.Close()

	return api.SwitchBlockOff(c.target)
}

const enableDoc = `
Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by disabling certain sets of commands from successful execution.
Disabled commands must be manually enabled to proceed.

Some commands offer a ` + "`--force`" + ` option that can be used to bypass a block.
` + commandSets + `
`

const enableExamples = `
To allow the model to be destroyed:

    juju enable-command destroy-model

To allow the machines, applications, units and relations to be removed:

    juju enable-command remove-object

To allow changes to the model:

    juju enable-command all
`
