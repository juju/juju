// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/rpc/params"
)

var usageExposeSummary = `
Makes an application publicly available over the network.`[1:]

var usageExposeDetails = `
Adjusts the firewall rules and any relevant security mechanisms of the
cloud to allow public access to the application.

If no additional options are specified, the command will, by default, allow
access from ` + "`0.0.0.0/0`" + ` to all ports opened by the application. For example, to
expose all ports opened by apache2, you can run:

    juju expose apache2

The ` + "`--endpoints`" + ` option may be used to restrict the effect of this command to
the list of ports opened for a comma-delimited list of endpoints. For instance,
to only expose the ports opened by ` + "`apache2`" + ` for the ` + "`www`" + ` endpoint, you can run:

    juju expose apache2 --endpoints www

To make the selected set of ports accessible by specific CIDRs, the ` + "`--to-cidrs`" + `
option may be used with a comma-delimited list of CIDR values. For example:

    juju expose apache2 --to-cidrs 10.0.0.0/24,192.168.1.0/24

To make the selected set of ports accessible by specific spaces, the ` + "`--to-spaces`" + `
option may be used with a comma-delimited list of space names. For example:

    juju expose apache2 --to-spaces public

All of the above options can be combined together. In addition, multiple ` + "`juju expose`" + `
invocations can be used to specify granular expose rules for different
endpoints. For example, to allow access to all opened apache ports from
` + "`0.0.0.0/0`" + ` but restrict access to any port opened for the ` + "`logs`" + ` endpoint to
CIDR ` + "`10.0.0.0/24`" + ` you can run:

    juju expose apache2
    juju expose apache2 --endpoints logs --to-cidrs 10.0.0.0/24

Each ` + "`juju expose`" + ` invocation always overwrites any previous expose rule for
the same endpoint name. For example, running the following commands instruct
juju to only allow access to ports opened for the ` + "`logs`" + ` endpoint from CIDR
` + "`192.168.0.0/24`" + `.

    juju expose apache2 --endpoints logs --to-cidrs 10.0.0.0/24
    juju expose apache2 --endpoints logs --to-cidrs 192.168.0.0/24

`[1:]

const example = `
To expose an application:

    juju expose apache2

To expose an application to one or multiple spaces:

    juju expose apache2 --to-spaces public

To expose an application to one or multiple endpoints:

    juju expose apache2 --endpoints logs

To expose an application to one or multiple CIDRs:

    juju expose apache2 --to-cidrs 10.0.0.0/24
`

// NewExposeCommand returns a command to expose applications.
func NewExposeCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&exposeCommand{})
}

// exposeCommand is responsible exposing applications.
type exposeCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName      string
	ExposedEndpointsList string
	ExposeToSpacesList   string
	ExposeToCIDRsList    string
}

func (c *exposeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "expose",
		Args:     "<application name>",
		Purpose:  usageExposeSummary,
		Doc:      usageExposeDetails,
		Examples: example,
		SeeAlso: []string{
			"unexpose",
		},
	})
}

func (c *exposeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.ExposedEndpointsList, "endpoints", "", "Expose only the ports that charms have opened for this comma-delimited list of endpoints")
	f.StringVar(&c.ExposeToSpacesList, "to-spaces", "", "A comma-delimited list of spaces that should be able to access the application ports once exposed")
	f.StringVar(&c.ExposeToCIDRsList, "to-cidrs", "", "A comma-delimited list of CIDRs that should be able to access the application ports once exposed")
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
	Unexpose(applicationName string, exposedEndpoints []string) error
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

	exposedEndpoints := c.buildExposedEndpoints()
	return block.ProcessBlockedError(client.Expose(c.ApplicationName, exposedEndpoints), block.BlockChange)
}

func (c *exposeCommand) buildExposedEndpoints() map[string]params.ExposedEndpoint {
	endpoints := splitCommaDelimitedList(c.ExposedEndpointsList)
	spaces := splitCommaDelimitedList(c.ExposeToSpacesList)
	cidrs := splitCommaDelimitedList(c.ExposeToCIDRsList)

	if len(endpoints)+len(spaces)+len(cidrs) == 0 {
		// No granular expose params required
		return nil
	}

	var allNetworkCIDRCount int
	for _, cidr := range cidrs {
		if cidr == firewall.AllNetworksIPV4CIDR || cidr == firewall.AllNetworksIPV6CIDR {
			allNetworkCIDRCount++
		}
	}

	if len(endpoints) == 0 && len(spaces) == 0 && len(cidrs) == allNetworkCIDRCount {
		// No granular expose params required; this is equivalent
		// to "juju expose <application>"
		return nil
	}

	expDetails := make(map[string]params.ExposedEndpoint)
	if len(endpoints) == 0 {
		// If no endpoints are specified, this applies to all ("") endpoints
		endpoints = append(endpoints, "")
	}

	for _, epName := range endpoints {
		expDetails[epName] = params.ExposedEndpoint{
			ExposeToSpaces: spaces,
			ExposeToCIDRs:  cidrs,
		}
	}

	return expDetails
}

func splitCommaDelimitedList(list string) []string {
	var items []string
	for _, token := range strings.Split(list, ",") {
		token = strings.TrimSpace(token)
		if len(token) == 0 {
			continue
		}
		items = append(items, token)
	}
	return items
}
