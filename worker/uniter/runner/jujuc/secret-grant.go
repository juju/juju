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

type secretGrantCommand struct {
	cmd.CommandBase
	ctx Context

	uri  string
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
Access is granted in the context of a relation - unless revoked
earlier, once the relation is removed, so too is the access grant.

By default, all units of the related application are granted access.
Optionally specify a unit name to limit access to just that unit.

Examples:
    secret-grant secret:9m4e2mr0ui3e8a215n4g --unit mediawiki/6 
    secret-grant secret:9m4e2mr0ui3e8a215n4g --relation db:2
`
	return jujucmd.Info(&cmd.Info{
		Name:    "secret-grant",
		Args:    "<ID>",
		Purpose: "grant access to a secret",
		Doc:     doc,
	})
}

// SetFlags implements cmd.Command.
func (c *secretGrantCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.unit, "unit", "", "the unit to grant access")
	f.Var(c.relationIdProxy, "r", "the relation with which to associate the grant")
	f.Var(c.relationIdProxy, "relation", "the relation with which to associate the grant")
}

// Init implements cmd.Command.
func (c *secretGrantCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing secret URI")
	}
	c.uri = args[0]
	if _, err := secrets.ParseURI(c.uri); err != nil {
		return errors.Trace(err)
	}
	if c.relationId == -1 {
		return errors.Errorf("no relation id specified")
	}
	if c.unit != "" && !names.IsValidUnit(c.unit) {
		return errors.NotValidf("unit %q", c.unit)
	}
	if c.unit == "" {
		return cmd.CheckEmpty(args[1:])
	}

	remoteAppName, err := remoteAppForRelation(c.ctx, c.unit)
	if err != nil {
		return errors.Trace(err)
	}
	if remoteAppName != "" {
		c.app = remoteAppName
	}
	return cmd.CheckEmpty(args[1:])
}

func remoteAppForRelation(ctx Context, unitName string) (string, error) {
	remoteAppName, err := ctx.RemoteApplicationName()
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	if remoteAppName == "" {
		remoteUnitName, err := ctx.RemoteUnitName()
		if err != nil && !errors.IsNotFound(err) {
			return "", errors.Trace(err)
		}
		remoteAppName, _ = names.UnitApplication(remoteUnitName)
	}
	if remoteAppName != "" && unitName != "" {
		appNameForUnit, _ := names.UnitApplication(unitName)
		if appNameForUnit != remoteAppName {
			return "", errors.Errorf("cannot specify unit %q in relation to application %q", unitName, remoteAppName)
		}
	}
	return remoteAppName, nil
}

// Run implements cmd.Command.
func (c *secretGrantCommand) Run(_ *cmd.Context) error {
	r, err := c.ctx.Relation(c.relationId)
	if err != nil {
		return errors.Trace(err)
	}
	key := r.RelationTag().Id()
	args := &SecretGrantRevokeArgs{
		RelationKey: &key,
	}
	if c.unit != "" {
		args.UnitName = &c.unit
	}
	if c.app != "" {
		args.ApplicationName = &c.app
	}

	return c.ctx.GrantSecret(c.uri, args)
}
