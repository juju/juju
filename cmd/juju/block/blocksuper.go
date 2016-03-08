// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"
)

const superBlockCmdDoc = `

Juju allows to safeguard deployed models from unintentional damage by preventing
execution of operations that could alter model.

This is done by blocking certain commands from successful execution. Blocked commands
must be manually unblocked to proceed.

"juju block" is used to list or to enable model blocks in
 the Juju model.
`

const superBlockCmdPurpose = "list and enable model blocks"

// Command is the top-level command wrapping all storage functionality.
type Command struct {
	cmd.SuperCommand
}

// NewSuperBlockCommand creates the block supercommand and
// registers the subcommands that it supports.
func NewSuperBlockCommand() cmd.Command {
	blockcmd := Command{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "block",
				Doc:         superBlockCmdDoc,
				UsagePrefix: "juju",
				Purpose:     superBlockCmdPurpose,
			})}
	blockcmd.Register(newDestroyCommand())
	blockcmd.Register(newRemoveCommand())
	blockcmd.Register(newChangeCommand())
	blockcmd.Register(newListCommand())
	return &blockcmd
}
