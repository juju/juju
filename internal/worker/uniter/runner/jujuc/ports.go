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
	Purpose: "register a request to open a port or port range",
	Doc: `
open-port registers a request to open the specified port or port range.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.
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
	Purpose: "register a request to close a port or port range",
	Doc: `
close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.
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
