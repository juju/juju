// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.juju.block")

// ProtectionCommand is a super for environment protection commands that block/unblock operations.
type ProtectionCommand struct {
	envcmd.EnvCommandBase
	operation string
	desc      string
}

// BlockClientAPI defines the client API methods that the protection command uses.
type ClientAPI interface {
	Close() error
	EnvironmentSet(config map[string]interface{}) error
}

var getBlockClientAPI = func(p *ProtectionCommand) (ClientAPI, error) {
	return p.NewAPIClient()
}

var (
	// This variable has all valid operations that can be
	// supplied to the command.
	// These operations do not necessarily correspond to juju commands
	// but are rather juju command groupings.
	blockArgs = []string{"destroy-environment", "remove-object", "all-changes"}

	// Formatted representation of block command valid arguments.
	blockArgsFmt = fmt.Sprintf(strings.Join(blockArgs, " | "))
)

// setBlockEnvironmentVariable sets desired environment variable to given value.
func (p *ProtectionCommand) setBlockEnvironmentVariable(block bool) error {
	client, err := getBlockClientAPI(p)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	attrs := map[string]interface{}{config.BlockKeyPrefix + p.operation: block}
	return client.EnvironmentSet(attrs)
}

// assignValidOperation verifies that supplied operation is supported.
func (p *ProtectionCommand) assignValidOperation(cmd string, args []string) error {
	if len(args) != 1 {
		return errors.Trace(errors.Errorf("must specify one of [%v] to %v", blockArgsFmt, cmd))
	}
	var err error
	p.operation, err = p.obtainValidArgument(args[0])
	return err
}

// obtainValidArgument returns polished argument:
// it checks that the argument is a supported operation and
// forces it into lower case for consistency.
func (p *ProtectionCommand) obtainValidArgument(arg string) (string, error) {
	for _, valid := range blockArgs {
		if strings.EqualFold(valid, arg) {
			return strings.ToLower(arg), nil
		}
	}
	return "", errors.Trace(errors.Errorf("%q is not a valid argument: use one of [%v]", arg, blockArgsFmt))
}

// BlockCommand blocks specified operation.
type BlockCommand struct {
	ProtectionCommand
}

var blockDoc = `

Juju allows to safeguard deployed environments from unintentional damage by preventing
execution of operations that could alter environment.

This is done by blocking certain commands from successful execution. Blocked commands
must be manually unblocked to proceed.

Some comands offer a --force option that can be used to bypass a block.

Commands that can be blocked are grouped based on logical operations as follows:

destroy-environment includes command:
    destroy-environment

remove-object includes termination commands:
    destroy-environment
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
    destroy-environment
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
   To prevent the environment from being destroyed:
   juju block destroy-environment

   To prevent the machines, services, units and relations from being removed:
   juju block remove-object

   To prevent changes to the environment:
   juju block all-changes

See Also:
   juju help unblock

`

// Info provides information about command.
// Satisfying Command interface.
func (c *BlockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "block",
		Args:    blockArgsFmt,
		Purpose: "block an operation that would alter a running environment",
		Doc:     blockDoc,
	}
}

// Init initializes the command.
// Satisfying Command interface.
func (c *BlockCommand) Init(args []string) error {
	return c.assignValidOperation("block", args)
}

// Run blocks commands from running successfully.
// Satisfying Command interface.
func (c *BlockCommand) Run(_ *cmd.Context) error {
	return c.setBlockEnvironmentVariable(true)
}

// Block describes block type
type Block int8

const (
	// BlockDestroy describes the block that
	// blocks destroy- commands
	BlockDestroy Block = iota

	// BlockRemove describes the block that
	// blocks remove- commands
	BlockRemove

	// BlockChange describes the block that
	// blocks change commands
	BlockChange
)

var blockedMessages = map[Block]string{
	BlockDestroy: destroyMsg,
	BlockRemove:  removeMsg,
	BlockChange:  changeMsg,
}

// ProcessBlockedError ensures that correct and user-friendly message is
// displayed to the user based on the block type.
func ProcessBlockedError(err error, block Block) error {
	if params.IsCodeOperationBlocked(err) {
		logger.Errorf(blockedMessages[block])
		return cmd.ErrSilent
	}
	if err != nil {
		return err
	}
	return nil
}

var removeMsg = `
All operations that remove (or delete or terminate) machines, services, units or
relations have been blocked for the current environment.
To unblock removal, run

    juju unblock remove-object

`
var destroyMsg = `
destroy-environment operation has been blocked for the current environment.
To remove the block run

    juju unblock destroy-environment

`
