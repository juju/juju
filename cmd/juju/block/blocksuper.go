// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

const superBlockCmdDoc = `

Juju allows to safeguard deployed environments from unintentional damage by preventing
execution of operations that could alter environment.

This is done by blocking certain commands from successful execution. Blocked commands
must be manually unblocked to proceed.

"juju block" is used to list or to enable environment blocks in
 the Juju environment.
`

const superBlockCmdPurpose = "list and enable environment blocks"

// Command is the top-level command wrapping all storage functionality.
type Command struct {
	cmd.SuperCommand
}

// NewBlockCommand creates the block supercommand and
// registers the subcommands that it supports.
func NewBlockCommand() cmd.Command {
	blockcmd := Command{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "block",
				Doc:         superBlockCmdDoc,
				UsagePrefix: "juju",
				Purpose:     superBlockCmdPurpose,
			})}
	blockcmd.Register(envcmd.Wrap(&DestroyBlockCommand{}))
	blockcmd.Register(envcmd.Wrap(&RemoveBlockCommand{}))
	blockcmd.Register(envcmd.Wrap(&ChangeBlockCommand{}))
	blockcmd.Register(envcmd.Wrap(&ListCommand{}))
	return &blockcmd
}
