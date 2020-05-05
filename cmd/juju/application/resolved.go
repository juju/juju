// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewResolvedCommand() cmd.Command {
	return modelcmd.Wrap(&resolvedCommand{})
}

// resolvedCommand marks a unit in an error state as ready to continue.
type resolvedCommand struct {
	modelcmd.ModelCommandBase
	applicationResolveAPI applicationResolveAPI
	clientAPI             clientAPI

	UnitNames []string
	NoRetry   bool
	All       bool
}

func (c *resolvedCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "resolved",
		Args:    "[<unit> ...]",
		Purpose: "Marks unit errors resolved and re-executes failed hooks.",
		Aliases: []string{"resolve"},
	})
}

func (c *resolvedCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.NoRetry, "no-retry", false, "Do not re-execute failed hooks on the unit")
	f.BoolVar(&c.All, "all", false, "Marks all units in error as resolved")
}

func (c *resolvedCommand) Init(args []string) error {
	if c.All {
		if len(args) > 0 {
			return errors.NotSupportedf("specifying unit names(s) with --all")
		}
		return nil
	}
	if len(args) > 0 {
		c.UnitNames = args
		for _, u := range args {
			if !names.IsValidUnit(u) {
				return errors.NotValidf("unit name %q", u)
			}
		}
	} else {
		return errors.Errorf("no unit specified")
	}
	return nil
}

type applicationResolveAPI interface {
	Close() error
	BestAPIVersion() int
	ResolveUnitErrors(units []string, all, retry bool) error
}

type clientAPI interface {
	Resolved(unit string, retry bool) error
	Close() error
}

func (c *resolvedCommand) getapplicationResolveAPI() (applicationResolveAPI, error) {
	if c.applicationResolveAPI != nil {
		return c.applicationResolveAPI, nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *resolvedCommand) getClientAPI() (clientAPI, error) {
	if c.clientAPI != nil {
		return c.clientAPI, nil
	}
	return c.NewAPIClient()
}

func (c *resolvedCommand) Run(ctx *cmd.Context) error {
	applicationResolveAPI, err := c.getapplicationResolveAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer applicationResolveAPI.Close()

	if applicationResolveAPI.BestAPIVersion() >= 6 {
		return block.ProcessBlockedError(applicationResolveAPI.ResolveUnitErrors(c.UnitNames, c.All, !c.NoRetry), block.BlockChange)
	}

	if c.All {
		return errors.Errorf("resolving all units or more than one unit not support by this version of Juju")
	}

	clientAPI, err := c.getClientAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer clientAPI.Close()
	for _, unit := range c.UnitNames {
		if len(c.UnitNames) > 1 {
			fmt.Fprintf(ctx.GetStdout(), "resolving unit %q\n", unit)
		}
		if err := block.ProcessBlockedError(clientAPI.Resolved(unit, !c.NoRetry), block.BlockChange); err != nil {
			return errors.Annotatef(err, "error resolving unit %q", unit)
		}
	}
	if len(c.UnitNames) > 1 {
		fmt.Fprintln(ctx.GetStdout(), "all units resolved")
	}
	return nil
}
