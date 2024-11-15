// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
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

	UnitNames []string
	NoRetry   bool
	All       bool
}

const resolvedCommandExamples = `

	juju resolved mysql/0

	juju resolved mysql/0 mysql/1

	juju resolved --all
`

func (c *resolvedCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "resolved",
		Args:     "[<unit> ...]",
		Purpose:  "Marks unit errors resolved and re-executes failed hooks.",
		Aliases:  []string{"resolve"},
		Examples: resolvedCommandExamples,
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
	ResolveUnitErrors(ctx context.Context, units []string, all, retry bool) error
}

func (c *resolvedCommand) getapplicationResolveAPI(ctx context.Context) (applicationResolveAPI, error) {
	if c.applicationResolveAPI != nil {
		return c.applicationResolveAPI, nil
	}

	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *resolvedCommand) Run(ctx *cmd.Context) error {
	applicationResolveAPI, err := c.getapplicationResolveAPI(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer applicationResolveAPI.Close()

	return block.ProcessBlockedError(ctx, applicationResolveAPI.ResolveUnitErrors(ctx, c.UnitNames, c.All, !c.NoRetry), block.BlockChange)
}
