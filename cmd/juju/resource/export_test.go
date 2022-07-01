// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/jujuclient/jujuclienttesting"
)

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
	c := CharmResourcesCommand{
		baseCharmResourcesCommand{
			CreateResourceListerFn: func(schema string, deps ResourceListerDependencies) (ResourceLister, error) {
				return resourceLister, nil
			},
		},
	}
	c.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(&c)
}

func NewUploadCommandForTest(newClient func() (UploadClient, error), filesystem modelcmd.Filesystem) *UploadCommand {
	cmd := &UploadCommand{newClient: newClient}
	cmd.SetFilesystem(filesystem)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}

func NewListCommandForTest(newClient func() (ListClient, error)) *ListCommand {
	cmd := &ListCommand{newClient: newClient}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return cmd
}
