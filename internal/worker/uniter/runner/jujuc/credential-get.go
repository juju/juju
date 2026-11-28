// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
)

// CredentialGetCommand implements the leader-get command.
type CredentialGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

// NewCredentialGetCommand returns a new CredentialGetCommand with the given context.
func NewCredentialGetCommand(ctx Context) (cmd.Command, error) {
	return &CredentialGetCommand{ctx: ctx}, nil
}

// Info is part of the cmd.Command interface.
func (c *CredentialGetCommand) Info() *cmd.Info {
	doc := `
Returns cloud connection details to a unit of a trusted application.
In a machine model, the result includes the model's credential when one
is configured. In a Kubernetes model, it includes the in-cluster API
endpoint and CA certificate, plus the unit's service-account token as
an OAuth2 credential. Keep output that contains credentials private.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/
`
	examples := `
Read the cloud connection details as JSON from a trusted charm hook:

    credential-get --format=json
`
	return jujucmd.Info(&cmd.Info{
		Name:     "credential-get",
		Args:     "",
		Purpose:  "Returns cloud connection details for a trusted application.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *CredentialGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Init is part of the cmd.Command interface.
func (c *CredentialGetCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// Run is part of the cmd.Command interface.
func (c *CredentialGetCommand) Run(ctx *cmd.Context) error {
	credential, err := c.ctx.CloudSpec(ctx)
	if err != nil {
		return errors.Annotatef(err, "cannot access cloud credentials")
	}
	return c.out.Write(ctx, credential)
}
