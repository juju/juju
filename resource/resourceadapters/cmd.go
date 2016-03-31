// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/resource/cmd"
)

// CharmCmdBase is an adapter for charmcmd.CommandBase.
type CharmCmdBase struct {
	*charmcmd.CommandBase
}

// Connect implements cmd.CommandBase.
func (c *CharmCmdBase) Connect(context *jujucmd.Context) (cmd.CharmResourceLister, error) {
	lister, err := c.CommandBase.Connect(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lister, nil
}
