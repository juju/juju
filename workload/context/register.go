// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"
)

const RegisterCmdName = "payload-register"

type RegisterCmd struct {
	Comp  Component
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
}

// SetFlags implements cmd.Command.
func (c *RegisterCmd) SetFlags(_ *gnuflag.FlagSet) {}

// Run implements cmd.Command.
func (c *RegisterCmd) Run(ctx *cmd.Context) error {
	info := workload.Info{
		Workload: charm.Workload{
			Name: c.class,
			Type: c.typ,
		},
		Status: workload.Status{
			State: workload.StatusRunning,
		},
		Details: workload.Details{
			ID: c.id,
		},
	}
	if err := c.Comp.Track(info); err != nil {
		return errors.Trace(err)
	}

	return nil
}
