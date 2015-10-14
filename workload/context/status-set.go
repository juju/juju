// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/workload"
)

// StatusSetCmdName is the name of the payload status-set command.
const StatusSetCmdName = "payload-status-set"

// NewStatusSetCmd returns a new StatusSetCmd that wraps the given context.
func NewStatusSetCmd(ctx HookContext) (*StatusSetCmd, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't tracked properly.
		return nil, errors.Trace(err)
	}
	return &StatusSetCmd{api: compCtx}, nil
}

// StatusSetCmd is a command that registers a payload with juju.
type StatusSetCmd struct {
	cmd.CommandBase

	api    Component
	id     string
	status string
}

// Info implements cmd.Command.
func (c StatusSetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    StatusSetCmdName,
		Args:    "<status> <id>",
		Purpose: "update the status of a payload",
		Doc: `
"payload-status-set" is used while a hook (update-status) is running to update the
current status of a registered payload. The <id> provided must match a payload that
has been previously registered with juju using payload-register. The <status> must
be on of the follow: starting, started, stopping, stopped
`,
	}
}

// Init implements cmd.Command.
func (c *StatusSetCmd) Init(args []string) error {
	if len(args) < 2 {
		return errors.Errorf("missing required arguments")
	}
	c.id = args[1]
	c.status = args[0]
	return nil
}

// Run implements cmd.Command.
func (c *StatusSetCmd) Run(ctx *cmd.Context) error {
	if err := c.validate(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.api.SetStatus(c.status, c.id); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immedeiately so that status reflects the
	// workload correctly.
	if err := c.api.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *StatusSetCmd) validate(ctx *cmd.Context) error {
	switch c.status {
	case workload.StateStarting:
	case workload.StateRunning:
	case workload.StateStopping:
	case workload.StateStopped:
		return nil
	}
	return errors.Errorf("incorrect status, must be one of the following: starting, started, stopping, stopped")
}
