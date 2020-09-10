// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageExposeSummary = `
Makes an application publicly available over the network.`[1:]

var usageExposeDetails = `
Adjusts the firewall rules and any relevant security mechanisms of the
cloud to allow public access to the application.

Examples:
    juju expose wordpress

See also: 
    unexpose`[1:]

// NewExposeCommand returns a command to expose applications.
func NewExposeCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&exposeCommand{})
}

// exposeCommand is responsible exposing applications.
type exposeCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
}

func (c *exposeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "expose",
		Args:    "<application name>",
		Purpose: usageExposeSummary,
		Doc:     usageExposeDetails,
	})
}

func (c *exposeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.ApplicationName = args[0]
	return cmd.CheckEmpty(args[1:])
}

type applicationExposeAPI interface {
	Close() error
	Expose(applicationName string, exposedEndpoints map[string]params.ExposedEndpoint) error
	Unexpose(applicationName string) error
}

func (c *exposeCommand) getAPI() (applicationExposeAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Run changes the juju-managed firewall to expose any
// ports that were also explicitly marked by units as open.
func (c *exposeCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.Expose(c.ApplicationName, nil), block.BlockChange)
}
