// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// DoCommand enqueues an Action for running on the given unit with given
// params
type DoCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const doDoc = `
Queue an Action for execution on a given unit, with a given set of params.
Params are validated according to the charm for the unit's service.  The 
valid params can be seen using "juju action defined <service>".

Params must be in a yaml file which is passed with the --params flag.
`

func (c *DoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "do",
		Args:    "<unit> [--params <filename>.yml]",
		Purpose: "TODO: queue an action for execution",
		Doc:     doDoc,
	}
}
