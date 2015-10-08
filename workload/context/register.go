// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

// RegisterCmdName is the name of the payload register command.
const RegisterCmdName = "payload-register"

// NewRegisterCmd returns a new RegisterCmd that wraps the given context.
func NewRegisterCmd(ctx HookContext) (*RegisterCmd, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't tracked properly.
		return nil, errors.Trace(err)
	}
	return &RegisterCmd{api: compCtx}, nil
}

// RegisterCmd is a command that registers a payload with juju.
type RegisterCmd struct {
	cmd.CommandBase

	api   Component
	typ   string
	class string
	id    string
	tags  []string
}

// Info implements cmd.Command.
func (c RegisterCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    RegisterCmdName,
		Args:    "<type> <class> <id> [tags...]",
		Purpose: "register a charm payload with juju",
		Doc: `
"payload-register" is used while a hook is running to let Juju know that a
payload has been started. The information used to start the payload must be
provided when "register" is run.

The payload class must correspond to one of the payloads defined in
the charm's metadata.yaml.

		`,
	}
}

// Init implements cmd.Command.
func (c *RegisterCmd) Init(args []string) error {
	if len(args) < 3 {
		return errors.Errorf("missing required arguments")
	}
	c.typ = args[0]
	c.class = args[1]
	c.id = args[2]
	c.tags = args[3:]
	return nil
}

// Run implements cmd.Command.
func (c *RegisterCmd) Run(ctx *cmd.Context) error {
	if err := c.validate(ctx); err != nil {
		return errors.Trace(err)
	}
	info := workload.Info{
		Workload: charm.Workload{
			Name: c.class,
			Type: c.typ,
		},
		Status: workload.Status{
			State: workload.StateRunning,
		},
		Details: workload.Details{
			ID: c.id,
			Status: workload.PluginStatus{
				State: workload.StateRunning,
			},
		},
	}
	if err := c.api.Track(info); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immedeiately so that status reflects the
	// workload correctly.
	if err := c.api.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *RegisterCmd) validate(ctx *cmd.Context) error {
	meta, err := readMetadata(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	found := false
	for _, class := range meta.PayloadClasses {
		if c.class == class.Name {
			if c.typ != class.Type {
				return errors.Errorf("incorrect type %q for payload %q, expected %q", c.typ, class.Name, class.Type)
			}
			found = true
		}
	}
	if !found {
		return errors.Errorf("payload %q not found in metadata.yaml", c.class)
	}
	return nil
}
