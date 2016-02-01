// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewUnblockCommand returns a new command that removes the block from
// the specified operation.
func NewUnblockCommand() cmd.Command {
	return modelcmd.Wrap(&unblockCommand{})
}

// unblockCommand removes the block from desired operation.
type unblockCommand struct {
	modelcmd.ModelCommandBase
	operation string
	client    UnblockClientAPI
}

var (
	unblockDoc = `

Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by blocking certain commands from successful execution. Blocked commands
must be manually unblocked to proceed.

Some commands offer a --force option that can be used to bypass a block.

Commands that can be unblocked are grouped based on logical operations as follows:

destroy-model includes command:
    destroy-model

remove-object includes termination commands:
    destroy-model
    remove-machine
    remove-relation
    remove-service
    remove-unit

all-changes includes all alteration commands
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
   To allow the model to be destroyed:
   juju unblock destroy-model

   To allow the machines, services, units and relations to be removed:
   juju unblock remove-object

   To allow changes to the model:
   juju unblock all-changes

See Also:
   juju help block
`

	// blockArgsFmt has formatted representation of block command valid arguments.
	blockArgsFmt = fmt.Sprintf(strings.Join(blockArgs, " | "))
)

// assignValidOperation verifies that supplied operation is supported.
func (p *unblockCommand) assignValidOperation(cmd string, args []string) error {
	if len(args) < 1 {
		return errors.Trace(errors.Errorf("must specify one of [%v] to %v", blockArgsFmt, cmd))
	}
	var err error
	p.operation, err = p.obtainValidArgument(args[0])
	return err
}

// obtainValidArgument returns polished argument:
// it checks that the argument is a supported operation and
// forces it into lower case for consistency.
func (p *unblockCommand) obtainValidArgument(arg string) (string, error) {
	for _, valid := range blockArgs {
		if strings.EqualFold(valid, arg) {
			return strings.ToLower(arg), nil
		}
	}
	return "", errors.Trace(errors.Errorf("%q is not a valid argument: use one of [%v]", arg, blockArgsFmt))
}

// Info provides information about command.
// Satisfying Command interface.
func (c *unblockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unblock",
		Args:    blockArgsFmt,
		Purpose: "unblock an operation that would alter a running model",
		Doc:     unblockDoc,
	}
}

// Init initializes the command.
// Satisfying Command interface.
func (c *unblockCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.Trace(errors.New("can only specify block type"))
	}

	return c.assignValidOperation("unblock", args)
}

// SetFlags implements Command.SetFlags.
func (c *unblockCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Run unblocks previously blocked commands.
// Satisfying Command interface.
func (c *unblockCommand) Run(_ *cmd.Context) error {
	client := c.client
	if client == nil {
		client, err := getBlockAPI(&c.ModelCommandBase)
		if err != nil {
			return errors.Trace(err)
		}
		defer client.Close()
	}

	return client.SwitchBlockOff(TypeFromOperation(c.operation))
}

// UnblockClientAPI defines the client API methods that unblock command uses.
type UnblockClientAPI interface {
	Close() error
	SwitchBlockOff(blockType string) error
}

var getUnblockClientAPI = func(p *unblockCommand) (UnblockClientAPI, error) {
	return getBlockAPI(&p.ModelCommandBase)
}
