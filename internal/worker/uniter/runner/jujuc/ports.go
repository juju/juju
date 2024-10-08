// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/network"
)

const (
	portFormat = "<port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp"
)

// portCommand implements the open-port and close-port commands.
type portCommand struct {
	cmd.CommandBase
	info       *cmd.Info
	action     func(*portCommand) error
	portRange  network.PortRange
	endpoints  string
	formatFlag string // deprecated

}

func (c *portCommand) Info() *cmd.Info {
	return jujucmd.Info(c.info)
}

func (c *portCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
	f.StringVar(&c.endpoints, "endpoints", "", "a comma-delimited list of application endpoints to target with this operation")
}

func (c *portCommand) Init(args []string) error {
	if args == nil {
		return errors.Errorf("no port or range specified")
	}

	portRange, err := network.ParsePortRange(strings.ToLower(args[0]))
	if err != nil {
		return errors.Trace(err)
	}
	c.portRange = portRange

	return cmd.CheckEmpty(args[1:])
}

func (c *portCommand) Run(ctx *cmd.Context) error {
	if c.formatFlag != "" {
		fmt.Fprintf(ctx.Stderr, "--format flag deprecated for command %q", c.Info().Name)
	}
	return c.action(c)
}

var openPortInfo = &cmd.Info{
	Name:    "open-port",
	Args:    portFormat,
	Purpose: "Register a request to open a port or port range.",
	Doc: `
open-port registers a request to open the specified port or port range.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.

The behavior differs a little bit between machine charms and Kubernetes charms.

Machine charms
On public clouds the port will only be open while the application is exposed.
It accepts a single port or range of ports with an optional protocol, which
may be icmp, udp, or tcp. tcp is the default.

open-port will not have any effect if the application is not exposed, and may
have a somewhat delayed effect even if it is. This operation is transactional,
so changes will not be made unless the hook exits successfully.

Prior to Juju 2.9, when charms requested a particular port range to be opened,
Juju would automatically mark that port range as opened for all defined
application endpoints. As of Juju 2.9, charms can constrain opened port ranges
to a set of application endpoints by providing the --endpoints flag followed by
a comma-delimited list of application endpoints.

Kubernetes charms
The port will open directly regardless of whether the application is exposed or not.
This connects to the fact that juju expose currently has no effect on sidecar charms.
Additionally, it is currently not possible to designate a range of ports to open for
Kubernetes charms; to open a range, you will have to run open-port multiple times.
`,
	Examples: `
    # Open port 80 to TCP traffic:
    open-port 80/tcp

    # Open port 1234 to UDP traffic:
    open-port 1234/udp

    # Open a range of ports to UDP traffic:
    open-port 1000-2000/udp

    # Open a range of ports to TCP traffic for specific
    # application endpoints (since Juju 2.9):
    open-port 1000-2000/tcp --endpoints dmz,monitoring
`,
}

func NewOpenPortCommand(ctx Context) (cmd.Command, error) {
	return &portCommand{
		info:   openPortInfo,
		action: makePortRangeCommand(ctx.OpenPortRange),
	}, nil
}

var closePortInfo = &cmd.Info{
	Name:    "close-port",
	Args:    portFormat,
	Purpose: "Register a request to close a port or port range.",
	Doc: `
close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.
`,
	Examples: `
    # Close single port
    close-port 80

    # Close a range of ports
    close-port 9000-9999/udp

    # Disable ICMP
    close-port icmp

    # Close a range of ports for a set of endpoints (since Juju 2.9)
    close-port 80-90 --endpoints dmz,public
`,
}

func NewClosePortCommand(ctx Context) (cmd.Command, error) {
	return &portCommand{
		info:   closePortInfo,
		action: makePortRangeCommand(ctx.ClosePortRange),
	}, nil
}

func makePortRangeCommand(op func(string, network.PortRange) error) func(*portCommand) error {
	return func(c *portCommand) error {
		// Operation applies to all endpoints
		if c.endpoints == "" {
			return op("", c.portRange)
		}

		for _, endpoint := range strings.Split(c.endpoints, ",") {
			endpoint = strings.TrimSpace(endpoint)
			if err := op(endpoint, c.portRange); err != nil {
				return errors.Trace(err)
			}
		}

		return nil
	}
}
