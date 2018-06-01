// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

func ListCharmResourcesCommandChannel(c modelcmd.Command) string {
	return modelcmd.InnerCommand(c).(*ListCharmResourcesCommand).channel
}

func CharmResourcesCommandChannel(c modelcmd.Command) string {
	return modelcmd.InnerCommand(c).(*CharmResourcesCommand).channel
}

func ListCommandTarget(c *ListCommand) string {
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

var FormatApplicationResources = formatApplicationResources

func NewCharmResourcesCommandForTest(resourceLister ResourceLister) modelcmd.ModelCommand {
	var c CharmResourcesCommand
	c.setResourceLister(resourceLister)
	c.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(&c)
}

func NewListCharmResourcesCommandForTest(resourceLister ResourceLister) modelcmd.ModelCommand {
	var c ListCharmResourcesCommand
	c.setResourceLister(resourceLister)
	c.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(&c)
}

func NewUploadCommandForTest(deps UploadDeps) *UploadCommand {
	cmd := &UploadCommand{deps: deps}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}

func NewListCommandForTest(deps ListDeps) *ListCommand {
	cmd := &ListCommand{deps: deps}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}
