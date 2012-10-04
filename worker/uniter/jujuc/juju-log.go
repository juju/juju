package jujuc

import (
	"errors"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"strings"
)

// JujuLogCommand implements the juju-log command.
type JujuLogCommand struct {
	*HookContext
	Message string
	Debug   bool
	Level   string // unused
}

func NewJujuLogCommand(ctx *HookContext) (cmd.Command, error) {
	return &JujuLogCommand{HookContext: ctx}, nil
}

func (c *JujuLogCommand) Info() *cmd.Info {
	return &cmd.Info{"juju-log", "<message>", "write a message to the juju log", ""}
}

func (c *JujuLogCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.BoolVar(&c.Debug, "debug", false, "log at debug level")
	f.StringVar(&c.Level, "l", "INFO", "Send log message at the given level")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if args == nil {
		return errors.New("no message specified")
	}
	c.Message = strings.Join(args, " ")
	return nil
}

func (c *JujuLogCommand) Run(_ *cmd.Context) error {
	s := []string{c.Unit.Name()}
	if c.RelationId != -1 {
		s = append(s, c.envRelationId())
	}
	msg := strings.Join(s, " ") + ": " + c.Message
	if c.Debug {
		log.Debugf("%s", msg)
	} else {
		log.Printf("%s", msg)
	}
	return nil
}
