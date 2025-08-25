// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewDisableCommand returns a disable-command command instance
// that will use the default API.
func NewDisableCommand() cmd.Command {
	return modelcmd.Wrap(&disableCommand{
		apiFunc: func(c newAPIRoot) (blockClientAPI, error) {
			return getBlockAPI(c)
		},
	})
}

type disableCommand struct {
	modelcmd.ModelCommandBase
	apiFunc func(newAPIRoot) (blockClientAPI, error)
	target  string
	message string
}

// Init implements Command.
func (c *disableCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing command set (%s)", validTargets)
	}
	c.target, args = args[0], args[1:]
	target, ok := toAPIValue[c.target]
	if !ok {
		return errors.Errorf("bad command set, valid options: %s", validTargets)
	}
	c.target = target
	c.message = strings.Join(args, " ")
	return nil
}

// Info implements Command.
func (c *disableCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "disable-command",
		Args:     "<command set> [message...]",
		Purpose:  "Disables commands for the model.",
		Doc:      disableCommandDoc,
		Examples: disableCommandExamples,
		SeeAlso: []string{
			"disabled-commands",
			"enable-command",
		},
	})
}

type blockClientAPI interface {
	Close() error
	SwitchBlockOn(blockType, msg string) error
}

// Run implements Command.Run
func (c *disableCommand) Run(ctx *cmd.Context) error {
	api, err := c.apiFunc(c)
	if err != nil {
		return errors.Annotate(err, "cannot connect to the API")
	}
	defer api.Close()

	return api.SwitchBlockOn(c.target, c.message)
}

var disableCommandDoc = `
Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by disabling certain sets of commands from successful execution.
Disabled commands must be manually enabled to proceed.

Some commands offer a ` + "`--force`" + ` option that can be used to bypass the disabling.
` + commandSets

const disableCommandExamples = `
To prevent the model from being destroyed:

    juju disable-command destroy-model "Check with SA before destruction."

To prevent the machines, applications, units and relations from being removed:

    juju disable-command remove-object

To prevent changes to the model:

    juju disable-command all "Model locked down"
`
