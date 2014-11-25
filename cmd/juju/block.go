// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
)

// ProtectionCommand is a super for environment protection commands that block/unblock operations.
type ProtectionCommand struct {
	envcmd.EnvCommandBase
	operation string
	desc      string
}

// BlockClientAPI defines the client API methods that the protection command uses.
type BlockClientAPI interface {
	Close() error
	EnvironmentSet(config map[string]interface{}) error
}

var getBlockClientAPI = func(p *ProtectionCommand) (BlockClientAPI, error) {
	return p.NewAPIClient()
}

var (
	// This variable has all valid operations that can be
	// supplied to the command.
	// These operations do not necessarily correspond to juju commands
	// but are rather juju command groupings.
	blockArgs = []string{"destroy-environment", "remove-object"}

	// Formatted representation of block command valid arguments
	blockArgsFmt = fmt.Sprintf(strings.Join(blockArgs, " | "))
)

// setBlockEnvironmentVariable sets desired environment variable to given value
func (p *ProtectionCommand) setBlockEnvironmentVariable(block bool) error {
	client, err := getBlockClientAPI(p)
	if err != nil {
		return err
	}
	defer client.Close()
	attrs := map[string]interface{}{config.BlockKeyPrefix + p.operation: block}
	return client.EnvironmentSet(attrs)
}

// assignValidOperation verifies that supplied operation is supported.
func (p *ProtectionCommand) assignValidOperation(cmd string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("must specify one of [%v] to %v", blockArgsFmt, cmd)
	}
	var err error
	p.operation, err = p.obtainValidArgument(args[0])
	return err
}

// obtainValidArgument returns polished argument:
// it checks that the argument is a supported operation and
// forces it into lower case for consistency
func (p *ProtectionCommand) obtainValidArgument(arg string) (string, error) {
	for _, valid := range blockArgs {
		if strings.EqualFold(valid, arg) {
			return strings.ToLower(arg), nil
		}
	}
	return "", fmt.Errorf("%q is not a valid argument: use one of [%v]", arg, blockArgsFmt)
}

// BlockCommand blocks specified operation.
type BlockCommand struct {
	ProtectionCommand
}

var blockDoc = `

Juju allows to safeguard deployed environments from unintentional damage by preventing
execution of operations that could alter environment.

This is done by blocking certain operations from successful execution. Blocked operations
must be manually unblocked to proceed.

Operations that can be blocked are

destroy environment


Examples:
   juju block destroy-environment      (blocks destroy environment)

See Also:
   juju help unblock
`

func (c *BlockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "block",
		Args:    blockArgsFmt,
		Purpose: "block an operation that would alter a running environment",
		Doc:     blockDoc,
	}
}

func (c *BlockCommand) Init(args []string) error {
	return c.assignValidOperation("block", args)
}

func (c *BlockCommand) Run(_ *cmd.Context) error {
	return c.setBlockEnvironmentVariable(true)
}

type BlockableRemoveCommand struct {
	envcmd.EnvCommandBase
}

func (c *BlockableRemoveCommand) processBlockedError(err error) error {
	if params.IsCodeOperationBlocked(err) {
		logger.Errorf(blockedRemoveObjectMsg, c.ConnectionName())
		return cmd.ErrSilent
	}
	return err
}

var blockedRemoveObjectMsg = `
All operations that remove (or delete or terminate) machines, services, units or relations have been blocked for environment %q.
To unblock removal, run

    juju unblock remove-object

`
