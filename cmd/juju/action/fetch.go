// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import "github.com/juju/cmd"

// FetchCommand retrieves the returned result for an Action.
type FetchCommand struct {
	ActionCommandBase
	undefinedActionCommand
}

const fetchDoc = `
Retrieve the returned result for an Action by identifier.

ex.

juju action fetch action:UUID
`

func (c *FetchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "fetch",
		Args:    "<action identifier>",
		Purpose: "TODO: retrieve the results of an action",
		Doc:     fetchDoc,
	}
}
