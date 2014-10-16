// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const (
	portFormat = "<port>[/<protocol>] or <from>-<to>[/<protocol>]"

	portExp  = "(?:[0-9]+)"
	protoExp = "(?:[a-z0-9]+)"
)

var validPortOrRange = regexp.MustCompile("^" + portExp + "(?:-" + portExp + ")?(/" + protoExp + ")?$")

type port struct {
	number   int
	protocol string
}

func (p port) validate() error {
	if p.number < 1 || p.number > 65535 {
		return errors.Errorf(`port must be in the range [1, 65535]; got "%v"`, p.number)
	}
	proto := strings.ToLower(p.protocol)
	if proto != "tcp" && proto != "udp" {
		return errors.Errorf(`protocol must be "tcp" or "udp"; got %q`, p.protocol)
	}
	return nil
}

type portRange struct {
	fromPort, toPort int
	protocol         string
}

func (pr portRange) validate() error {
	if pr.fromPort == pr.toPort {
		return port{pr.fromPort, pr.protocol}.validate()
	}
	if pr.fromPort > pr.toPort {
		return errors.Errorf(
			"invalid port range %d-%d/%s; expected fromPort <= toPort",
			pr.fromPort, pr.toPort, pr.protocol,
		)
	}
	if pr.fromPort < 1 || pr.fromPort > 65535 {
		return errors.Errorf(`fromPort must be in the range [1, 65535]; got "%v"`, pr.fromPort)
	}
	if pr.toPort < 1 || pr.toPort > 65535 {
		return errors.Errorf(`toPort must be in the range [1, 65535]; got "%v"`, pr.toPort)
	}
	proto := strings.ToLower(pr.protocol)
	if proto != "tcp" && proto != "udp" {
		return errors.Errorf(`protocol must be "tcp" or "udp"; got %q`, pr.protocol)
	}
	return nil
}

func parseArguments(args []string) (portRange, error) {
	arg := strings.ToLower(args[0])
	if !validPortOrRange.MatchString(arg) {
		return portRange{}, errors.Errorf("expected %s; got %q", portFormat, args[0])
	}
	portOrRange := validPortOrRange.FindString(arg)
	parts := strings.SplitN(portOrRange, "/", 2)

	protocol := "tcp"
	if len(parts) > 1 {
		protocol = parts[1]
	}
	ports := parts[0]
	portParts := strings.SplitN(ports, "-", 2)
	fromPort, toPort := 0, 0
	if len(portParts) >= 1 {
		port, err := strconv.Atoi(portParts[0])
		if err != nil {
			return portRange{}, errors.Annotatef(err, "expected port number; got %q", portParts[0])
		}
		fromPort = port
	}
	if len(portParts) == 2 {
		port, err := strconv.Atoi(portParts[1])
		if err != nil {
			return portRange{}, errors.Annotatef(err, "expected port number; got %q", portParts[1])
		}
		toPort = port
	} else {
		toPort = fromPort
	}
	pr := portRange{fromPort, toPort, protocol}
	return pr, pr.validate()
}

// portCommand implements the open-port and close-port commands.
type portCommand struct {
	cmd.CommandBase
	info       *cmd.Info
	action     func(*portCommand) error
	Protocol   string
	FromPort   int
	ToPort     int
	formatFlag string // deprecated
}

func (c *portCommand) Info() *cmd.Info {
	return c.info
}

func (c *portCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *portCommand) Init(args []string) error {
	if args == nil {
		return errors.Errorf("no port or range specified")
	}

	portRange, err := parseArguments(args)
	if err != nil {
		return errors.Trace(err)
	}

	c.FromPort = portRange.fromPort
	c.ToPort = portRange.toPort
	c.Protocol = portRange.protocol
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
	Doc:     "The port range will only be open while the service is exposed.",
}

func NewOpenPortCommand(ctx Context) cmd.Command {
	return &portCommand{
		info: openPortInfo,
		action: func(c *portCommand) error {
			return ctx.OpenPorts(c.Protocol, c.FromPort, c.ToPort)
		},
	}
}

var closePortInfo = &cmd.Info{
	Name:    "close-port",
	Args:    portFormat,
	Purpose: "ensure a port or range is always closed",
}

func NewClosePortCommand(ctx Context) cmd.Command {
	return &portCommand{
		info: closePortInfo,
		action: func(c *portCommand) error {
			return ctx.ClosePorts(c.Protocol, c.FromPort, c.ToPort)
		},
	}
}
