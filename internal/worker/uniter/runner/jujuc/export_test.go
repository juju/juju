// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
)

func HandleSettingsFile(c *RelationSetCommand, ctx *cmd.Context) error {
	return c.handleSettingsFile(ctx)
}

func NewJujuLogCommandWithMocks(ctx JujuLogContext) cmd.Command {
	return &JujuLogCommand{
		ctx: ctx,
	}
}

func NewJujucCommandWrappedForTest(c cmd.Command) cmd.Command {
	return &cmdWrapper{c, nil}
}
