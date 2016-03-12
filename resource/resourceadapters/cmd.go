// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/resource/cmd"
)

// CharmCmdBase is an adapter for charmcmd.CommandBase.
type CharmCmdBase struct {
	*charmcmd.CommandBase
}

// Connect implements cmd.CommandBase.
func (c *CharmCmdBase) Connect() (cmd.CharmResourceLister, error) {
	return c.CommandBase.Connect()
}
