// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"os"
	"path/filepath"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/internal/charm"
)

// NewPayloadRegisterCmd returns a new PayloadRegisterCmd that wraps the given context.
func NewPayloadRegisterCmd(ctx Context) (cmd.Command, error) {
	return &PayloadRegisterCmd{ctx: ctx}, nil
}

// PayloadRegisterCmd is a command that registers a payload with juju.
type PayloadRegisterCmd struct {
	cmd.CommandBase
	ctx ContextPayloads

	typ    string
	class  string
	id     string
	labels []string
}

// TODO(ericsnow) Change "tags" to "labels" in the help text?

// Info implements cmd.Command.
func (c PayloadRegisterCmd) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "payload-register",
		Args:    "<type> <class> <id> [tags...]",
		Purpose: "Register a charm payload with Juju.",
		Doc: `
"payload-register" is used while a hook is running to let Juju know that a
payload has been started. The information used to start the payload must be
provided when "register" is run.

The payload class must correspond to one of the payloads defined in
the charm's metadata.yaml.

An example fragment from metadata.yaml:

payloads:
    monitoring:
        type: docker
    kvm-guest:
        type: kvm

`,
		Examples: `
    payload-register monitoring docker 0fcgaba
`,
	})
}

// Init implements cmd.Command.
func (c *PayloadRegisterCmd) Init(args []string) error {
	if len(args) < 3 {
		return errors.Errorf("missing required arguments")
	}
	c.typ = args[0]
	c.class = args[1]
	c.id = args[2]
	c.labels = args[3:]
	return nil
}

// Run implements cmd.Command.
func (c *PayloadRegisterCmd) Run(ctx *cmd.Context) error {
	if err := c.validate(ctx); err != nil {
		return errors.Trace(err)
	}
	pl := payloads.Payload{
		PayloadClass: charm.PayloadClass{
			Name: c.class,
			Type: c.typ,
		},
		ID:     c.id,
		Status: payloads.StateRunning,
		Labels: c.labels,
		Unit:   "a-application/0", // TODO(ericsnow) eliminate this!
	}
	if err := c.ctx.TrackPayload(pl); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immediately so that status reflects the
	// payload correctly.
	if err := c.ctx.FlushPayloads(ctx); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Print out the full ID.

	return nil
}

func (c *PayloadRegisterCmd) validate(ctx *cmd.Context) error {
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

func readMetadata(ctx *cmd.Context) (*charm.Meta, error) {
	filename := filepath.Join(ctx.Dir, "metadata.yaml")
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()
	meta, err := charm.ReadMeta(file)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return meta, nil
}
