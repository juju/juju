// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
)

type secretRevokeCommand struct {
	cmd.CommandBase
	ctx Context

	name string
	app  string
	unit string
}

// NewSecretRevokeCommand returns a command to revoke access to a secret.
func NewSecretRevokeCommand(ctx Context) (cmd.Command, error) {
	return &secretRevokeCommand{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c *secretRevokeCommand) Info() *cmd.Info {
	doc := `
Revoke access to view the value of a specified secret.
Access may be revoked from an application (all units of
that application lose access), or from a specified unit.

Examples:
    secret-revoke password --app mediawiki
    secret-revoke password --unit mediawiki/6
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-revoke",
		Args:    "<name>",
		Purpose: "revoke access to a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretRevokeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.app, "app", "", "the application to revoke access")
	f.StringVar(&c.app, "application", "", "")
	f.StringVar(&c.unit, "unit", "", "the unit to revoke access")
}

// Init implements cmd.Command.
func (c *secretRevokeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret name")
	}
	if c.app == "" && c.unit == "" {
		return errors.New("missing application or unit")
	}
	if c.app != "" && !names.IsValidApplication(c.app) {
		return errors.NotValidf("application %q", c.app)
	}
	if c.unit != "" && !names.IsValidUnit(c.unit) {
		return errors.NotValidf("unit %q", c.unit)
	}
	c.name = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run implements cmd.Command.
func (c *secretRevokeCommand) Run(_ *cmd.Context) error {
	args := &SecretGrantRevokeArgs{}
	if c.app != "" {
		args.ApplicationName = &c.app
	}
	if c.unit != "" {
		args.UnitName = &c.unit
	}

	return c.ctx.RevokeSecret(c.name, args)
}
