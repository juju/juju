// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// UnblockCommand removes the block from desired operation.
type UnblockCommand struct {
	ProtectionCommand
}

var (
	// unblockDocEnding unblock doc ending
	unblockDocEnding = `

Examples:
   To allow the environment to be destroyed:
   juju unblock destroy-environment

   To allow the machines, services, units and relations to be removed:
   juju unblock remove-object

   To allow changes to the environment:
   juju unblock all-changes

See Also:
   juju help block
`
	// blockDoc formatted block doc
	unblockDoc = fmt.Sprintf(blockBaseDoc, "unblocked", unblockDocEnding)
)

// Info provides information about command.
// Satisfying Command interface.
func (c *UnblockCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unblock",
		Args:    blockArgsFmt,
		Purpose: "unblock an operation that would alter a running environment",
		Doc:     unblockDoc,
	}
}

// Init initializes the command.
// Satisfying Command interface.
func (c *UnblockCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.Trace(errors.New("can only specify block type"))
	}

	return c.assignValidOperation("unblock", args)
}

// Run unblocks previously blocked commands.
// Satisfying Command interface.
func (c *UnblockCommand) Run(_ *cmd.Context) error {
	client, err := getUnblockClientAPI(c)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	return client.SwitchBlockOff(TranslateOperation(c.operation))
}

// UnblockClientAPI defines the client API methods that unblock command uses.
type UnblockClientAPI interface {
	Close() error
	SwitchBlockOff(blockType string) error
}

var getUnblockClientAPI = func(p *UnblockCommand) (UnblockClientAPI, error) {
	return p.NewBlockAPI()
}
