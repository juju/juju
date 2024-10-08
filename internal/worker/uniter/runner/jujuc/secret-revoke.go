// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretRevokeCommand struct {
	cmd.CommandBase
	ctx Context

	secretURL *secrets.URI
	app       string
	unit      string

	relationId      int
	relationIdProxy gnuflag.Value
}

// NewSecretRevokeCommand returns a command to revoke access to a secret.
func NewSecretRevokeCommand(ctx Context) (cmd.Command, error) {
	cmd := &secretRevokeCommand{ctx: ctx}
	var err error
	cmd.relationIdProxy, err = NewRelationIdValue(ctx, &cmd.relationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cmd, nil
}

// Info implements cmd.Command.
func (c *secretRevokeCommand) Info() *cmd.Info {
	doc := `
Revoke access to view the value of a specified secret.
Access may be revoked from an application (all units of
that application lose access), or from a specified unit.
If run in a relation hook, the related application's 
access is revoked, unless a uni is specified, in which
case just that unit's access is revoked.'
`
	examples := `
    secret-revoke secret:9m4e2mr0ui3e8a215n4g
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --relation 1
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --app mediawiki
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6
`
	return jujucmd.Info(&cmd.Info{
		Name:     "secret-revoke",
		Args:     "<ID>",
		Purpose:  "Revoke access to a secret.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags implements cmd.Command.
func (c *secretRevokeCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.app, "app", "", "the application to revoke access")
	f.StringVar(&c.app, "application", "", "")
	f.StringVar(&c.unit, "unit", "", "the unit to revoke access")
	f.Var(c.relationIdProxy, "r", "the relation for which to revoke the grant")
	f.Var(c.relationIdProxy, "relation", "the relation for which to revoke the grant")
}

// Init implements cmd.Command.
func (c *secretRevokeCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	var err error
	if c.secretURL, err = secrets.ParseURI(args[0]); err != nil {
		return errors.Trace(err)
	}
	if c.app != "" {
		if !names.IsValidApplication(c.app) {
			return errors.NotValidf("application %q", c.app)
		}
	}
	if c.unit != "" {
		if !names.IsValidUnit(c.unit) {
			return errors.NotValidf("unit %q", c.unit)
		}
	}
	if c.app != "" && c.unit != "" {
		return errors.New("specify only one of application or unit")
	}
	if c.relationId >= 0 {
		r, err := c.ctx.Relation(c.relationId)
		if err != nil {
			return errors.Trace(err)
		}
		if c.app != "" {
			return errors.New("do not specify both relation and app")
		}
		c.app = r.RemoteApplicationName()
	}
	if c.app == "" && c.unit == "" {
		return errors.New("missing relation or application or unit")
	}
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

	return c.ctx.RevokeSecret(c.secretURL, args)
}
