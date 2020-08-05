// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
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
	formatFlag string // deprecated
}

func (c *portCommand) Info() *cmd.Info {
	return jujucmd.Info(c.info)
}

func (c *portCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
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
	Purpose: "register a port or range to open",
	Doc:     "The port range will only be open while the application is exposed.",
}

func NewOpenPortCommand(ctx Context) (cmd.Command, error) {
	return &portCommand{
		info: openPortInfo,
		action: func(c *portCommand) error {
			// TODO(achilleas): parse endpoints and pass them along;
			// for now emulate pre 2.9 behavior and open/close port
			// range for all endpoints.
			return ctx.OpenPortRange("", c.portRange)
		},
	}, nil
}

var closePortInfo = &cmd.Info{
	Name:    "close-port",
	Args:    portFormat,
	Purpose: "ensure a port or range is always closed",
}

func NewClosePortCommand(ctx Context) (cmd.Command, error) {
	return &portCommand{
		info: closePortInfo,
		action: func(c *portCommand) error {
			// TODO(achilleas): parse endpoints and pass them along;
			// for now emulate pre 2.9 behavior and open/close port
			// range for all endpoints.
			return ctx.ClosePortRange("", c.portRange)
		},
	}, nil
}
