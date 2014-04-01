// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

// JujuLogCommand implements the juju-log command.
type JujuLogCommand struct {
	cmd.CommandBase
	ctx        Context
	Message    string
	Debug      bool
	Level      string
	formatFlag string // deprecated
}

func NewJujuLogCommand(ctx Context) cmd.Command {
	return &JujuLogCommand{ctx: ctx}
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
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *JujuLogCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no message specified")
	}
	c.Message = strings.Join(args, " ")
	return nil
}

func (c *JujuLogCommand) Run(ctx *cmd.Context) error {
	if c.formatFlag != "" {
		fmt.Fprintf(ctx.Stderr, "--format flag deprecated for command %q", c.Info().Name)
	}
	logger := loggo.GetLogger(fmt.Sprintf("unit.%s.juju-log", c.ctx.UnitName()))

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
	if r, found := c.ctx.HookRelation(); found {
		prefix = r.FakeId() + ": "
	}

	logger.Logf(logLevel, "%s%s", prefix, c.Message)
	return nil
}
