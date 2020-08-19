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

func UploadCommandResourceValue(c *UploadCommand) (application, name, value string) {
	return c.resourceValue.application,
		c.resourceValue.name,
		c.resourceValue.value
}

func UploadCommandApplication(c *UploadCommand) string {
	return c.application
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

func NewUploadCommandForTest(deps UploadDeps, filesystem modelcmd.Filesystem) *UploadCommand {
	cmd := &UploadCommand{deps: deps}
	cmd.SetFilesystem(filesystem)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}

func NewListCommandForTest(deps ListDeps) *ListCommand {
	cmd := &ListCommand{deps: deps}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}
