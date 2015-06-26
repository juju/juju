// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
)

var CmdSuffix = cmdSuffix

func HandleSettingsFile(c *RelationSetCommand, ctx *cmd.Context) error {
	return c.handleSettingsFile(ctx)
}
