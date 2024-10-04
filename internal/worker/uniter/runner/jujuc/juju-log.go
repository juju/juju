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
	corelogger "github.com/juju/juju/core/logger"
)

// JujuLogContext is the Context for the JujuLogCommand
//
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/juju-log_mock.go github.com/juju/juju/internal/worker/uniter/runner/jujuc JujuLogContext
type JujuLogContext interface {
	UnitName() string
	HookRelation() (ContextRelation, error)
	GetLoggerByName(module string) corelogger.Logger
}

// JujuLogCommand implements the juju-log command.
type JujuLogCommand struct {
	cmd.CommandBase
	ctx        JujuLogContext
	Message    string
	Debug      bool
	Level      string
	formatFlag string // deprecated
}

func NewJujuLogCommand(ctx Context) (cmd.Command, error) {
	return &JujuLogCommand{ctx: ctx}, nil
}

func (c *JujuLogCommand) Info() *cmd.Info {
	examples := `
    juju-log -l 'WARN' Something has transpired
`
	return jujucmd.Info(&cmd.Info{
		Name:     "juju-log",
		Args:     "<message>",
		Purpose:  "Write a message to the juju log.",
		Examples: examples,
	})
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
	logger := c.ctx.GetLoggerByName(fmt.Sprintf("unit.%s.juju-log", c.ctx.UnitName()))

	logLevel := corelogger.INFO
	if c.Debug {
		logLevel = corelogger.DEBUG
	} else if c.Level != "" {
		var ok bool
		logLevel, ok = corelogger.ParseLevelFromString(c.Level)
		if !ok {
			logger.Warningf("Specified log level of %q is not valid", c.Level)
			logLevel = corelogger.INFO
		}
	}

	prefix := ""
	if r, err := c.ctx.HookRelation(); err == nil {
		prefix = r.FakeId() + ": "
	} else if errors.Is(err, errors.NotImplemented) {
		// if the hook relation is not implemented, then we want to continue
		// without a FakeId
	} else if !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	logger.Logf(logLevel, "%s%s", prefix, c.Message)
	return nil
}
