// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
)

// JujuLogCommand implements the juju-log command.
type JujuLogCommand struct {
	cmd.CommandBase
	ctx     Context
	Message string
	Debug   bool
	Level   string
}

func NewJujuLogCommand(ctx Context) (cmd.Command, error) {
	return &JujuLogCommand{ctx: ctx}, nil
}

func (c *JujuLogCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "juju-log",
		Args:    "<message>",
		Purpose: "write a message to the juju log",
	}
}

func (c *JujuLogCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Debug, "debug", false, "log at debug level")
	f.StringVar(&c.Level, "l", "INFO", "Send log message at the given level")
	f.StringVar(&c.Level, "log-level", "INFO", "")
}

func (c *JujuLogCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no message specified")
	}
	c.Message = strings.Join(args, " ")
	return nil
}

func (c *JujuLogCommand) Run(ctx *cmd.Context) error {
	logger := loggo.GetLogger(fmt.Sprintf("application.%s.juju-log", c.ctx.ApplicationName()))

	logLevel := loggo.INFO
	if c.Debug {
		logLevel = loggo.DEBUG
	} else if c.Level != "" {
		var ok bool
		logLevel, ok = loggo.ParseLevel(c.Level)
		if !ok {
			logger.Warningf("Specified log level of %q is not valid", c.Level)
			logLevel = loggo.INFO
		}
	}

	prefix := ""
	if r, err := c.ctx.HookRelation(); err == nil {
		prefix = r.FakeId() + ": "
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	logger.Logf(logLevel, "%s%s", prefix, c.Message)
	return nil
}
