// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/loggo"

	apiblock "github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/state/multiwatcher"
)

var logger = loggo.GetLogger("juju.cmd.juju.block")

// blockArgs has all valid operations that can be
// supplied to the command.
// These operations do not necessarily correspond to juju commands
// but are rather juju command groupings.
var blockArgs = []string{"destroy-environment", "remove-object", "all-changes"}

// TypeFromOperation translates given operation string
// such as destroy-environment, remove-object, etc to
// block type string as defined in multiwatcher.
var TypeFromOperation = func(operation string) string {
	for key, value := range blockTypes {
		if value == operation {
			return key
		}
	}
	panic(fmt.Sprintf("unknown operation %v", operation))
}

var blockTypes = map[string]string{
	string(multiwatcher.BlockDestroy): "destroy-environment",
	string(multiwatcher.BlockRemove):  "remove-object",
	string(multiwatcher.BlockChange):  "all-changes",
}

// OperationFromType translates given block type as
// defined in multiwatcher into the operation
// such as destroy-environment.
var OperationFromType = func(blockType string) string {
	return blockTypes[blockType]
}

// getBlockAPI returns a block api for block manipulation.
func getBlockAPI(c *envcmd.EnvCommandBase) (*apiblock.Client, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return apiblock.NewClient(root), nil
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
