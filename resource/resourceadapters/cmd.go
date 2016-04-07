// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"io"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/resource/cmd"
)

// CharmCmdBase is an adapter for charmcmd.CommandBase.
type CharmCmdBase struct {
	*charmcmd.CommandBase
}

// Connect implements cmd.CommandBase.
func (c *CharmCmdBase) Connect(ctx *jujucmd.Context) (cmd.CharmResourceLister, error) {
	client, closer, err := c.CommandBase.Connect(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return struct {
		charmstore.Client
		io.Closer
	}{client, closer}, nil
}
