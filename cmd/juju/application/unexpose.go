// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

var usageUnexposeSummary = `
Removes public availability over the network for an application.`[1:]

var usageUnexposeDetails = `
Adjusts the firewall rules and any relevant security mechanisms of the
cloud to deny public access to the application.

Applications are unexposed by default when they get created. If exposed via
the ` + "`juju expose`" + ` command, they can be unexposed by running the ` + "`juju unexpose`" + `
command.

If no additional options are specified, the command will unexpose the
application (if exposed).

The ` + "`--endpoints`" + ` option may be used to restrict the effect of this command to
the list of ports opened for a comma-delimited list of endpoints.

Note that when the ` + "`--endpoints`" + ` option is provided, the application will still
remain exposed if any other of its endpoints are still exposed. However, if
none of its endpoints remain exposed, the application will become unexposed.
`[1:]

// NewUnexposeCommand returns a command to unexpose applications.
func NewUnexposeCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&unexposeCommand{})
}

// unexposeCommand is responsible exposing applications.
type unexposeCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName      string
	ExposedEndpointsList string
}

const unexposeCommandExample = `
    juju unexpose apache2

To unexpose only the ports that charms have opened for the "www", or "www" and "logs" endpoints:

    juju unexpose apache2 --endpoints www

    juju unexpose apache2 --endpoints www,logs
`

func (c *unexposeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "unexpose",
		Args:     "<application name>",
		Purpose:  usageUnexposeSummary,
		Doc:      usageUnexposeDetails,
		Examples: unexposeCommandExample,
		SeeAlso: []string{
			"expose",
		},
	})
}

func (c *unexposeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.ExposedEndpointsList, "endpoints", "", "Unexpose only the ports that charms have opened for this comma-delimited list of endpoints")
}

func (c *unexposeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.ApplicationName = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *unexposeCommand) getAPI() (applicationExposeAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

// Run changes the juju-managed firewall to hide any
// ports that were also explicitly marked by units as closed.
func (c *unexposeCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	endpoints := splitCommaDelimitedList(c.ExposedEndpointsList)
	return block.ProcessBlockedError(client.Unexpose(c.ApplicationName, endpoints), block.BlockChange)
}
