// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

const portFormat = "<port>[/<protocol>]"

// portCommand implements the open-port and close-port commands.
type portCommand struct {
	cmd.CommandBase
	info       *cmd.Info
	action     func(*portCommand) error
	Protocol   string
	Port       int
	formatFlag string // deprecated
}

func (c *portCommand) Info() *cmd.Info {
	return c.info
}

func badPort(value interface{}) error {
	return fmt.Errorf(`port must be in the range [1, 65535]; got "%v"`, value)
}

func (c *portCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *portCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no port specified")
	}
	parts := strings.Split(args[0], "/")
	if len(parts) > 2 {
		return fmt.Errorf("expected %s; got %q", portFormat, args[0])
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil {
		return badPort(parts[0])
	}
	if port < 1 || port > 65535 {
		return badPort(port)
	}
	protocol := "tcp"
	if len(parts) == 2 {
		protocol = strings.ToLower(parts[1])
		if protocol != "tcp" && protocol != "udp" {
			return fmt.Errorf(`protocol must be "tcp" or "udp"; got %q`, protocol)
		}
	}
	c.Port = port
	c.Protocol = protocol
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
	Purpose: "register a port to open",
	Doc:     "The port will only be open while the service is exposed.",
}

func NewOpenPortCommand(ctx Context) cmd.Command {
	return &portCommand{
		info: openPortInfo,
		action: func(c *portCommand) error {
			return ctx.OpenPort(c.Protocol, c.Port)
		},
	}
}

var closePortInfo = &cmd.Info{
	Name:    "close-port",
	Args:    portFormat,
	Purpose: "ensure a port is always closed",
}

func NewClosePortCommand(ctx Context) cmd.Command {
	return &portCommand{
		info: closePortInfo,
		action: func(c *portCommand) error {
			return ctx.ClosePort(c.Protocol, c.Port)
		},
	}
}
