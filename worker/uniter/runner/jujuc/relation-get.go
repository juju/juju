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
	Application     bool

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

	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit id>"
	doc := `
relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

A unit can see its own settings by calling "relation-get - MYUNIT", this will include
any changes that have been made with "relation-set".

When reading remote relation data, a charm can call relation-get --app - to get
the data for the application data bag that is set by the remote applications
leader.
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
	f.Var(c.relationIdProxy, "r", "Specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")

	f.BoolVar(&c.Application, "app", false,
		`Get the relation data for the overall application, not just a unit`)
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

	if name, err := c.ctx.RemoteUnitName(); err == nil {
		c.UnitName = name
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	} else {
		// TODO(jam): 2019-10-03 implement RemoteApplicationName
		// name, err := c.ctx.RemoteApplicationName()
		if err == nil {
			c.UnitName = name
		} else if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
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
	if c.UnitName == c.ctx.UnitName() {
		var node Settings
		var err error
		if c.Application {
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
		if c.Application {
			settings, err = r.ReadApplicationSettings(c.UnitName)
		} else {
			settings, err = r.ReadSettings(c.UnitName)
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
