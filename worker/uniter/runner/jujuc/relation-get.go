// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	cmd.CommandBase
	ctx Context

	RelationId      int
	relationIdProxy gnuflag.Value
	relationContext gnuflag.Value

	Key      string
	UnitName string
	out      cmd.Output
}

func NewRelationGetCommand(ctx Context) (cmd.Command, error) {
	var err error
	cmd := &RelationGetCommand{ctx: ctx}
	cmd.relationIdProxy, err = NewRelationIdValue(ctx, &cmd.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmd.relationContext = NewEnumValue("", []string{"both", "unit", "application"})

	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit id>"
	doc := `
relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

A unit can see its own settings by calling "relation-get - MYUNIT", this will include
any changes that have been made with "relation-set". The default context when reading
your own setting is "unit", and "both" is not supported. Only the leader unit can
call "relation-get --context=application - MYUNIT".

When reading remote relation data, the default context is "both" and the per-unit
data will be overlayed on the application data.
`
	// There's nothing we can really do about the error here.
	if name, err := c.ctx.RemoteUnitName(); err == nil {
		args = "[<key> [<unit id>]]"
		doc += fmt.Sprintf("Current default unit id is %q.", name)
	} else if !errors.IsNotFound(err) {
		logger.Errorf("Failed to retrieve remote unit name: %v", err)
	}
	return jujucmd.Info(&cmd.Info{
		Name:    "relation-get",
		Args:    args,
		Purpose: "get relation settings",
		Doc:     doc,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *RelationGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(c.relationIdProxy, "r", "specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")

	f.Var(c.relationContext, "context",
		`Specify whether you want only "unit", "application", or "both" settings from relation data`)
}

// Init is part of the cmd.Command interface.
func (c *RelationGetCommand) Init(args []string) error {
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	name, err := c.ctx.RemoteUnitName()
	if err == nil {
		c.UnitName = name
	} else if cause := errors.Cause(err); !errors.IsNotFound(cause) {
		return errors.Trace(err)
	}
	if len(args) > 0 {
		c.UnitName = args[0]
		args = args[1:]
	}
	if c.UnitName == "" {
		return fmt.Errorf("no unit id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationGetCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}
	var settings params.Settings
	relationContext := c.relationContext.String()
	if c.UnitName == c.ctx.UnitName() {
		var node Settings
		var err error
		if relationContext == "both" {
			return errors.Errorf("merged relation data not supported for your own unit")
		} else if relationContext == "application" {
			node, err = r.ApplicationSettings()
		} else {
			node, err = r.Settings()
		}
		if err != nil {
			return err
		}
		settings = node.Map()
	} else {
		var err error
		if relationContext == "application" {
			settings, err = r.ReadApplicationSettings(c.UnitName)
		} else if relationContext == "unit" {
			settings, err = r.ReadSettings(c.UnitName)
		} else {
			settings, err = r.ReadApplicationSettings(c.UnitName)
			if err != nil {
				return err
			}
			unitSettings, err := r.ReadSettings(c.UnitName)
			if err != nil {
				return err
			}
			// Overlay Unit settings on top of Application settings
			for k, v := range unitSettings {
				settings[k] = v
			}
		}
		if err != nil {
			return err
		}
	}
	if c.Key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.Key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}
