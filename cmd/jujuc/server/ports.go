package server

import (
	"errors"
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
	"strconv"
	"strings"
)

const portFormat = "<port>[/<protocol>]"

// portCommand implements the open-port and close-port commands.
type portCommand struct {
	ctx      *Context
	info     func() *cmd.Info
	action   func(*state.Unit, string, int) error
	Protocol string
	Port     int
}

func (c *portCommand) Info() *cmd.Info {
	return c.info()
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
	unit, err := c.ctx.State.Unit(c.ctx.LocalUnitName)
	if err != nil {
		return err
	}
	return c.action(unit, c.Protocol, c.Port)
}

func NewOpenPortCommand(ctx *Context) (cmd.Command, error) {
	if err := ctx.checkUnitState(); err != nil {
		return nil, err
	}
	return &portCommand{
		ctx: ctx,
		info: func() *cmd.Info {
			return &cmd.Info{
				"open-port", portFormat, "register a port to open",
				"The port will only be open while the service is exposed.",
			}
		},
		action: func(unit *state.Unit, protocol string, port int) error {
			return unit.OpenPort(protocol, port)
		},
	}, nil
}

func NewClosePortCommand(ctx *Context) (cmd.Command, error) {
	if err := ctx.checkUnitState(); err != nil {
		return nil, err
	}
	return &portCommand{
		ctx: ctx,
		info: func() *cmd.Info {
			return &cmd.Info{
				"close-port", portFormat, "ensure a port is always closed", "",
			}
		},
		action: func(unit *state.Unit, protocol string, port int) error {
			return unit.ClosePort(protocol, port)
		},
	}, nil
}
