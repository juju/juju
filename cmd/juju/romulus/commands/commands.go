// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package commands provides functionality for registering all the romulus commands.
package commands

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/juju/romulus/agree"
	"github.com/juju/juju/cmd/juju/romulus/allocate"
	"github.com/juju/juju/cmd/juju/romulus/createbudget"
	"github.com/juju/juju/cmd/juju/romulus/listagreements"
	"github.com/juju/juju/cmd/juju/romulus/listbudgets"
	"github.com/juju/juju/cmd/juju/romulus/listplans"
	"github.com/juju/juju/cmd/juju/romulus/setbudget"
	"github.com/juju/juju/cmd/juju/romulus/setplan"
	"github.com/juju/juju/cmd/juju/romulus/showbudget"
	"github.com/juju/juju/cmd/juju/romulus/updateallocation"
	"github.com/juju/juju/cmd/modelcmd"
)

type commandRegister interface {
	Register(cmd.Command)
}

// RegisterAll registers all romulus commands with the
// provided command registry.
func RegisterAll(r commandRegister) {
	register := func(c cmd.Command) {
		switch c := c.(type) {
		case modelcmd.ModelCommand:
			r.Register(modelcmd.Wrap(c))
		case modelcmd.CommandBase:
			r.Register(modelcmd.WrapBase(c))
		default:
			r.Register(c)
		}

	}
	register(agree.NewAgreeCommand())
	register(listagreements.NewListAgreementsCommand())
	register(allocate.NewAllocateCommand())
	register(listbudgets.NewListBudgetsCommand())
	register(createbudget.NewCreateBudgetCommand())
	register(listplans.NewListPlansCommand())
	register(setbudget.NewSetBudgetCommand())
	register(setplan.NewSetPlanCommand())
	register(showbudget.NewShowBudgetCommand())
	register(updateallocation.NewUpdateAllocationCommand())
}
