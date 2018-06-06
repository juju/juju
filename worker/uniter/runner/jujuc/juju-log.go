// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
)

//go:generate mockgen -package jujuc -destination juju-log_mock_test.go github.com/juju/juju/worker/uniter/runner/jujuc JujuLogCommandLogger,JujuLogContext
type JujuLogCommandLogger interface {
	Logf(level loggo.Level, message string, args ...interface{})
	Warningf(message string, args ...interface{})
}

type JujuLogContext interface {
	UnitName() string
	HookRelation() (ContextRelation, error)
}

// JujuLogCommand implements the juju-log command.
type JujuLogCommand struct {
	cmd.CommandBase
	ctx        JujuLogContext
	Message    string
	Debug      bool
	Level      string
	formatFlag string // deprecated
	getLogger  func(string) JujuLogCommandLogger
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
	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *JujuLogCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no message specified")
	}
	c.Message = strings.Join(args, " ")
	if c.getLogger == nil {
		c.getLogger = func(s string) JujuLogCommandLogger {
			return loggo.GetLogger(s)
		}
	}
	return nil
}

func (c *JujuLogCommand) Run(ctx *cmd.Context) error {
	if c.formatFlag != "" {
		fmt.Fprintf(ctx.Stderr, "--format flag deprecated for command %q", c.Info().Name)
	}
	logger := c.getLogger(fmt.Sprintf("unit.%s.juju-log", c.ctx.UnitName()))

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
	} else if errors.IsNotSupported(err) {
		// nop
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	logger.Logf(logLevel, "%s%s", prefix, c.Message)
	return nil
}
