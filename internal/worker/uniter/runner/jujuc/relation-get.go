// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	cmd.CommandBase
	ctx Context

	RelationId      int
	relationIdProxy gnuflag.Value
	Application     bool

	Key           string
	UnitOrAppName string
	out           cmd.Output
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

Further details:
relation-get reads the settings of the local unit, or of any remote unit, in a given
relation (set with -r, defaulting to the current relation identifier, as in relation-set).
The first argument specifies the settings key, and the second the remote unit, which may
be omitted if a default is available (that is, when running a relation hook other
than -relation-broken).

If the first argument is omitted, a dictionary of all current keys and values will be
printed; all values are always plain strings without any interpretation. If you need to
specify a remote unit but want to see all settings, use - for the first argument.

The environment variable JUJU_REMOTE_UNIT stores the default remote unit.

You should never depend upon the presence of any given key in relation-get output.
Processing that depends on specific values (other than private-address) should be
restricted to -relation-changed hooks for the relevant unit, and the absence of a
remote unit’s value should never be treated as an error in the local unit.

In practice, it is common and encouraged for -relation-changed hooks to exit early,
without error, after inspecting relation-get output and determining the data is
inadequate; and for all other hooks to be resilient in the face of missing keys,
such that -relation-changed hooks will be sufficient to complete all configuration
that depends on remote unit settings.

Key value pairs for remote units that have departed remain accessible for the lifetime
of the relation.
`
	examples := `
    # Getting the settings of the default unit in the default relation is done with:
    $ relation-get
    username: jim
    password: "12345"

    # To get a specific setting from the default remote unit in the default relation
    $ relation-get username
    jim

    # To get all settings from a particular remote unit in a particular relation you
    $ relation-get -r database:7 - mongodb/5
    username: bob
    password: 2db673e81ffa264c
`
	// There's nothing we can really do about the error here.
	if name, err := c.ctx.RemoteUnitName(); err == nil {
		args = "[<key> [<unit id>]]"
		doc += fmt.Sprintf("Current default unit id is %q.", name)
	} else if !errors.Is(err, errors.NotFound) {
		logger.Errorf("Failed to retrieve remote unit name: %v", err)
	}
	return jujucmd.Info(&cmd.Info{
		Name:     "relation-get",
		Args:     args,
		Purpose:  "Get relation settings.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *RelationGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.Var(c.relationIdProxy, "r", "Specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")

	f.BoolVar(&c.Application, "app", false,
		`Get the relation data for the overall application, not just a unit`)
}

func (c *RelationGetCommand) determineUnitOrAppName(args *[]string) error {
	// The logic is as follows:
	// 1) If a user supplies a unit or app name, that overrides any default
	//  a) If they supply --app and a unit name, we turn that back into an application name
	//  b) note, if they *don't* supply --app, and they specify an app name, that should be an error
	// 2) If no unit/app is supplied then we look at our context
	//  a) If --app is specified, then we use the context app
	//  b) If --app is not specified, but we don't have a context unit but do have a context app
	//     then we set --app, and set the target as the app
	//  c) If we have a context unit, then that is used
	if len(*args) > 0 {
		userSupplied := (*args)[0]
		*args = (*args)[1:]
		if c.Application {
			if names.IsValidApplication(userSupplied) {
				c.UnitOrAppName = userSupplied
			} else if names.IsValidUnit(userSupplied) {
				appName, err := names.UnitApplication(userSupplied)
				if err != nil {
					// Shouldn't happen, as we just validated it is a valid unit name
					return errors.Trace(err)
				}
				c.UnitOrAppName = appName
			}
		} else {
			if !names.IsValidUnit(userSupplied) {
				if names.IsValidApplication(userSupplied) {
					return fmt.Errorf("expected unit name, got application name %q", userSupplied)
				}
				return fmt.Errorf("invalid unit name %q", userSupplied)
			}
			c.UnitOrAppName = userSupplied
		}
		return nil
	}
	if c.Application {
		name, err := c.ctx.RemoteApplicationName()
		if errors.Is(err, errors.NotFound) {
			return fmt.Errorf("no unit or application specified")
		} else if err != nil {
			return errors.Trace(err)
		}
		c.UnitOrAppName = name
		return nil
	}
	// No args, no flags, check if there is a Unit context, or an App context
	if name, err := c.ctx.RemoteUnitName(); err == nil {
		c.UnitOrAppName = name
		return nil
	} else if !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	// Unit name not found, look for app context

	if name, err := c.ctx.RemoteApplicationName(); err == nil {
		c.UnitOrAppName = name
		c.Application = true
		return nil
	} else if !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	// If we got this far, there is no default value to give and nothing was
	// supplied, so it is an error
	return errors.New("no unit or application specified")
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

	if err := c.determineUnitOrAppName(&args); err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationGetCommand) Run(ctx *cmd.Context) error {
	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}

	settingsReaderFn := c.readLocalUnitOrAppSettings

	getFromController, err := c.mustReadSettingsFromController()
	if err != nil {
		return errors.Trace(err)
	}
	if getFromController {
		settingsReaderFn = c.readRemoteUnitOrAppSettings
	}

	settings, err := settingsReaderFn(ctx, r)
	if err != nil {
		return err
	}

	if c.Key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.Key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}

func (c *RelationGetCommand) mustReadSettingsFromController() (bool, error) {
	localUnitName := c.ctx.UnitName()
	if c.UnitOrAppName == localUnitName {
		return false, nil
	}

	localAppName, _ := names.UnitApplication(c.ctx.UnitName())
	if c.UnitOrAppName == localAppName {
		isLeader, err := c.ctx.IsLeader()
		if err != nil {
			return false, errors.Annotate(err, "cannot determine leadership status")
		}

		// If we are the leader for the requested app, read from local
		// uniter context
		if isLeader {
			return false, nil
		}
	}

	// Delegate the read to the controller
	return true, nil
}

func (c *RelationGetCommand) readLocalUnitOrAppSettings(ctx context.Context, r ContextRelation) (params.Settings, error) {
	var (
		node Settings
		err  error
	)

	if c.Application {
		node, err = r.ApplicationSettings(ctx)
	} else {
		node, err = r.Settings(ctx)
	}
	if err != nil {
		return nil, err
	}

	return node.Map(), nil
}

func (c *RelationGetCommand) readRemoteUnitOrAppSettings(ctx context.Context, r ContextRelation) (params.Settings, error) {
	if !c.Application {
		return r.ReadSettings(ctx, c.UnitOrAppName)
	}

	return r.ReadApplicationSettings(ctx, c.UnitOrAppName)
}
