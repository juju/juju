// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// BlockCommand blocks specified operation.
type BlockCommand struct {
	ProtectionCommand
	desc string
}

var (
	// blockDocEnding - ending of block doc
	blockDocEnding = `

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
	// blockDoc formatted block doc
	blockDoc = fmt.Sprintf(blockBaseDoc, "blocked", blockDocEnding)
)

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
	if err := c.assignValidOperation("block", args); err != nil {
		return err
	}

	if len(args) > 2 {
		return errors.Trace(errors.New("can only specify block type and its message"))
	}

	if len(args) == 2 {
		c.desc = args[1]
	}
	return nil
}

// Run blocks commands from running successfully.
// Satisfying Command interface.
func (c *BlockCommand) Run(_ *cmd.Context) error {
	client, err := getBlockClientAPI(c)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	return client.SwitchBlockOn(TranslateOperation(c.operation), c.desc)
}

// BlockClientAPI defines the client API methods that block command uses.
type BlockClientAPI interface {
	Close() error
	SwitchBlockOn(blockType, msg string) error
}

var getBlockClientAPI = func(p *BlockCommand) (BlockClientAPI, error) {
	return p.NewBlockAPI()
}
