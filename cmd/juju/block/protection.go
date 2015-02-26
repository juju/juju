// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	apiblock "github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/state/multiwatcher"
)

var logger = loggo.GetLogger("juju.cmd.juju.block")

// ProtectionCommand is a super for environment protection commands that block/unblock operations.
type ProtectionCommand struct {
	envcmd.EnvCommandBase
	operation string
}

// NewBlockAPI returns a storage api for block.
func (c *ProtectionCommand) NewBlockAPI() (*apiblock.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return apiblock.NewClient(root), nil
}

var TranslateOperation = func(operation string) string {
	var bt multiwatcher.BlockType
	switch strings.ToLower(operation) {
	case "destroy-environment":
		bt = multiwatcher.BlockDestroy
	case "remove-object":
		bt = multiwatcher.BlockRemove
	case "all-changes":
		bt = multiwatcher.BlockChange
	default:
		panic(fmt.Sprintf("unknown operation %v", operation))
	}
	return fmt.Sprintf("%v", bt)
}

var (
	// blockArgs has all valid operations that can be
	// supplied to the command.
	// These operations do not necessarily correspond to juju commands
	// but are rather juju command groupings.
	blockArgs = []string{"destroy-environment", "remove-object", "all-changes"}

	// blockArgsFmt has formatted representation of block command valid arguments.
	blockArgsFmt = fmt.Sprintf(strings.Join(blockArgs, " | "))

	// blockBaseDoc common block doc
	blockBaseDoc = `

Juju allows to safeguard deployed environments from unintentional damage by preventing
execution of operations that could alter environment.

This is done by blocking certain commands from successful execution. Blocked commands
must be manually unblocked to proceed.

Some comands offer a --force option that can be used to bypass a block.

Commands that can be %s are grouped based on logical operations as follows:

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
    %s
`
)

// assignValidOperation verifies that supplied operation is supported.
func (p *ProtectionCommand) assignValidOperation(cmd string, args []string) error {
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
func (p *ProtectionCommand) obtainValidArgument(arg string) (string, error) {
	for _, valid := range blockArgs {
		if strings.EqualFold(valid, arg) {
			return strings.ToLower(arg), nil
		}
	}
	return "", errors.Trace(errors.Errorf("%q is not a valid argument: use one of [%v]", arg, blockArgsFmt))
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
	if err == nil {
		return nil
	}
	if params.IsCodeOperationBlocked(err) {
		logger.Errorf("\n%v%v", err, blockedMessages[block])
		return cmd.ErrSilent
	}
	return err
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
var changeMsg = `
All operations that change environment have been blocked for the current environment.
To unblock changes, run

    juju unblock all-changes

`
