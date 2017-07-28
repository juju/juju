// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/juju/cmd/modelcmd"
)

func ListCharmResourcesCommandChannel(c modelcmd.Command) string {
	return modelcmd.InnerCommand(c).(*ListCharmResourcesCommand).channel
}

func ShowServiceCommandTarget(c *ShowServiceCommand) string {
	return c.target
}

func UploadCommandResourceFile(c *UploadCommand) (service, name, filename string) {
	return c.resourceFile.service,
		c.resourceFile.name,
		c.resourceFile.filename
}

func UploadCommandService(c *UploadCommand) string {
	return c.service
}

var FormatServiceResources = formatServiceResources
