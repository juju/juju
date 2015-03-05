// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// BaseBlockCommand is base command for all
// commands that enable blocks.
type BaseBlockCommand struct {
	envcmd.EnvCommandBase
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
	c.EnvCommandBase.SetFlags(f)
}

// BlockClientAPI defines the client API methods that block command uses.
type BlockClientAPI interface {
	Close() error
	SwitchBlockOn(blockType, msg string) error
}

var getBlockClientAPI = func(p *BaseBlockCommand) (BlockClientAPI, error) {
	return getBlockAPI(&p.EnvCommandBase)
}

// DestroyCommand blocks destroy environment.
type DestroyCommand struct {
	BaseBlockCommand
}

var destroyBlockDoc = `

This command allows to block environment destruction. 

To disable the block, run unblock command - see "juju help unblock". 
To by-pass the block, run destroy-enviornment with --force option.

"juju block destroy-environment" only blocks destroy-environment command.
   
Examples:
   To prevent the environment from being destroyed:
   juju block destroy-environment

`

// Info provides information about command.
// Satisfying Command interface.
func (c *DestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "destroy-environment",
		Purpose: "block an operation that would destroy Juju environment",
		Doc:     destroyBlockDoc,
	}
}

// Satisfying Command interface.
func (c *DestroyCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}

// RemoveCommand blocks commands that remove juju objects.
type RemoveCommand struct {
	BaseBlockCommand
}

var removeBlockDoc = `

This command allows to block all operations that would remove an object 
from Juju environment.

To disable the block, run unblock command - see "juju help unblock". 
To by-pass the block, where available, run desired remove command with --force option.

"juju block remove-object" blocks these commands:
    destroy-environment
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
func (c *RemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-object",
		Purpose: "block an operation that would remove an object",
		Doc:     removeBlockDoc,
	}
}

// Satisfying Command interface.
func (c *RemoveCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}

// ChangeCommand blocks commands that may change environment.
type ChangeCommand struct {
	BaseBlockCommand
}

var changeBlockDoc = `

This command allows to block all operations that would alter
Juju environment.

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
    destroy-environment
    ensure-availability
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
    set-env
    sync-tools
    unexpose
    unset
    unset-env
    upgrade-charm
    upgrade-juju
    user add
    user change-password
    user disable
    user enable
   
Examples:
   To prevent changes to the environment:
   juju block all-changes

`

// Info provides information about command.
// Satisfying Command interface.
func (c *ChangeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "all-changes",
		Purpose: "block operations that could change Juju environment",
		Doc:     changeBlockDoc,
	}
}

// Satisfying Command interface.
func (c *ChangeCommand) Run(_ *cmd.Context) error {
	return c.internalRun(c.Info().Name)
}
