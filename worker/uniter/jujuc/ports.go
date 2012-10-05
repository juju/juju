package jujuc

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"strconv"
	"strings"
)

const portFormat = "<port>[/<protocol>]"

// portCommand implements the open-port and close-port commands.
type portCommand struct {
	*HookContext
	info     *cmd.Info
	action   func(*portCommand) error
	Protocol string
	Port     int
}

func (c *portCommand) Info() *cmd.Info {
	return c.info
}

func badPort(value interface{}) error {
	return fmt.Errorf(`port must be in the range [1, 65535]; got "%v"`, value)
}

func (c *portCommand) Init(f *gnuflag.FlagSet, args []string) error {
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
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

func (c *portCommand) Run(_ *cmd.Context) error {
	return c.action(c)
}

var openPortInfo = &cmd.Info{
	"open-port", portFormat, "register a port to open",
	"The port will only be open while the service is exposed.",
}

func NewOpenPortCommand(ctx *HookContext) (cmd.Command, error) {
	return &portCommand{
		HookContext: ctx,
		info:        openPortInfo,
		action: func(c *portCommand) error {
			return c.Unit.OpenPort(c.Protocol, c.Port)
		},
	}, nil
}

var closePortInfo = &cmd.Info{
	"close-port", portFormat, "ensure a port is always closed", "",
}

func NewClosePortCommand(ctx *HookContext) (cmd.Command, error) {
	return &portCommand{
		HookContext: ctx,
		info:        closePortInfo,
		action: func(c *portCommand) error {
			return c.Unit.ClosePort(c.Protocol, c.Port)
		},
	}, nil
}
