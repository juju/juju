// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	jujucmd "github.com/juju/juju/v3/cmd"
)

type secretGrantCommand struct {
	cmd.CommandBase
	ctx Context

	name string
	app  string
	unit string

	relationId      int
	relationIdProxy gnuflag.Value
}

// NewSecretGrantCommand returns a command to grant access to a secret.
func NewSecretGrantCommand(ctx Context) (cmd.Command, error) {
	cmd := &secretGrantCommand{ctx: ctx}
	var err error
	cmd.relationIdProxy, err = NewRelationIdValue(ctx, &cmd.relationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cmd, nil
}

// Info implements cmd.Command.
func (c *secretGrantCommand) Info() *cmd.Info {
	doc := `
Grant access to view the value of a specified secret.
Access may be granted to an application (all units of that application
have access), or to a specified unit. Unless revoked earlier, when the
allowed application or unit is removed, so too is the access grant.

A relation may also be specified. This ties the access to the life of  
that relation - when the relation is removed, so too is the access grant.

Examples:
    secret-grant password --app mediawiki
    secret-grant password --unit mediawiki/6 
    secret-grant password --app mediawiki --relation db:2
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-grant",
		Args:    "<name>",
		Purpose: "grant access to a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretGrantCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.app, "app", "", "the application to grant access")
	f.StringVar(&c.app, "application", "", "")
	f.StringVar(&c.unit, "unit", "", "the unit to grant access")
	f.Var(c.relationIdProxy, "r", "Specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "the relation with which to associate the grant")
}

// Init implements cmd.Command.
func (c *secretGrantCommand) Init(args []string) error {
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
func (c *secretGrantCommand) Run(_ *cmd.Context) error {
	args := &SecretGrantRevokeArgs{}
	if c.app != "" {
		args.ApplicationName = &c.app
	}
	if c.unit != "" {
		args.UnitName = &c.unit
	}
	if c.relationId != -1 {
		args.RelationId = &c.relationId
	}

	return c.ctx.GrantSecret(c.name, args)
}
