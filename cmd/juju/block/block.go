// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

// BaseBlockCommand is base command for all
// commands that enable blocks.
type BaseBlockCommand struct {
	modelcmd.ModelCommandBase
	desc string
}

// Init initializes the command.
// Satisfying Command interface.
func (c *BaseBlockCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.Trace(errors.New("can only specify block message"))
	}

	if len(args) == 1 {
		c.desc = args[0]
	}
	return nil
}

// internalRun blocks commands from running successfully.
func (c *BaseBlockCommand) internalRun(operation string) error {
	client, err := getBlockClientAPI(c)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	return client.SwitchBlockOn(TypeFromOperation(operation), c.desc)
}

// SetFlags implements Command.SetFlags.
func (c *BaseBlockCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// BlockClientAPI defines the client API methods that block command uses.
type BlockClientAPI interface {
	Close() error
	SwitchBlockOn(blockType, msg string) error
}

var getBlockClientAPI = func(p *BaseBlockCommand) (BlockClientAPI, error) {
	return getBlockAPI(&p.ModelCommandBase)
}

func newDestroyCommand() cmd.Command {
	return modelcmd.Wrap(&destroyCommand{})
}

// destroyCommand blocks destroy environment.
type destroyCommand struct {
	BaseBlockCommand
}

var destroyBlockDoc = `

This command allows to block model destruction.

To disable the block, run unblock command - see "juju help unblock". 
To by-pass the block, run destroy-model with --force option.

"juju block destroy-model" only blocks destroy-model command.
   
Examples:
   To prevent the model from being destroyed:
   juju block destroy-model

`

// Info provides information about command.
// Satisfying Command interface.
func (c *destroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-model",
		Purpose: "block an operation that would destroy Juju model",
		Doc:     destroyBlockDoc,
	}
}

// Satisfying Command interface.
func (c *destroyCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}

func newRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand blocks commands that remove juju objects.
type removeCommand struct {
	BaseBlockCommand
}

var removeBlockDoc = `

This command allows to block all operations that would remove an object 
from Juju model.

To disable the block, run unblock command - see "juju help unblock". 
To by-pass the block, where available, run desired remove command with --force option.

"juju block remove-object" blocks these commands:
    destroy-model
    remove-machine
    remove-relation
    remove-service
    remove-unit
   
Examples:
   To prevent the machines, services, units and relations from being removed:
   juju block remove-object

`

// Info provides information about command.
// Satisfying Command interface.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-object",
		Purpose: "block an operation that would remove an object",
		Doc:     removeBlockDoc,
	}
}

// Satisfying Command interface.
func (c *removeCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}

func newChangeCommand() cmd.Command {
	return modelcmd.Wrap(&changeCommand{})
}

// changeCommand blocks commands that may change environment.
type changeCommand struct {
	BaseBlockCommand
}

var changeBlockDoc = `

This command allows to block all operations that would alter
Juju model.

To disable the block, run unblock command - see "juju help unblock". 
To by-pass the block, where available, run desired remove command with --force option.

"juju block all-changes" blocks these commands:
    add-machine
    add-relation
    add-unit
    authorised-keys add
    authorised-keys delete
    authorised-keys import
    deploy
    destroy-model
    enable-ha
    expose
    remove-machine
    remove-relation
    remove-service
    remove-unit
    resolved
    retry-provisioning
    run
    set
    set-constraints
    set-model-config
    sync-tools
    unexpose
    unset
    unset-model-config
    upgrade-charm
    upgrade-juju
    add-user
    change-user-password
    disable-user
    enable-user
   
Examples:
   To prevent changes to the model:
   juju block all-changes

`

// Info provides information about command.
// Satisfying Command interface.
func (c *changeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "all-changes",
		Purpose: "block operations that could change Juju model",
		Doc:     changeBlockDoc,
	}
}

// Satisfying Command interface.
func (c *changeCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}
