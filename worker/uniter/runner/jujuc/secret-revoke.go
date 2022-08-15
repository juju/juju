// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/secrets"
)

type secretRevokeCommand struct {
	cmd.CommandBase
	ctx Context

	uri  string
	app  string
	unit string

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
access is revoked.

Examples:
    secret-revoke secret:9m4e2mr0ui3e8a215n4g
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --relation 1
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --app mediawiki
    secret-revoke secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-revoke",
		Args:    "<ID>",
		Purpose: "revoke access to a secret",
		Doc:     doc,
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
	c.uri = args[0]
	if _, err := secrets.ParseURI(c.uri); err != nil {
		return errors.Trace(err)
	}
	count := 0
	if c.relationId >= 0 {
		count++
	}
	if c.app != "" {
		count++
		if !names.IsValidApplication(c.app) {
			return errors.NotValidf("application %q", c.app)
		}
	}
	if c.unit != "" {
		count++
		if !names.IsValidUnit(c.unit) {
			return errors.NotValidf("unit %q", c.unit)
		}
	}
	if count == 0 {
		return errors.New("missing relation or application or unit")
	}
	if count != 1 {
		return errors.New("specify only one of relation or application or unit")
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
	if c.relationId >= 0 {
		r, err := c.ctx.Relation(c.relationId)
		if err != nil {
			return errors.Trace(err)
		}
		key := r.RelationTag().Id()
		args.RelationKey = &key
		remoteAppName, err := remoteAppForRelation(c.ctx, c.unit)
		if err != nil {
			return errors.Trace(err)
		}
		if remoteAppName != "" {
			args.ApplicationName = &remoteAppName
		}
	}

	return c.ctx.RevokeSecret(c.uri, args)
}
