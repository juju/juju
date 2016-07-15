// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package commands provides functionality for registering all the romulus commands.
package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"

	"github.com/juju/romulus/cmd/agree"
	"github.com/juju/romulus/cmd/allocate"
	"github.com/juju/romulus/cmd/createbudget"
	"github.com/juju/romulus/cmd/listagreements"
	"github.com/juju/romulus/cmd/listbudgets"
	"github.com/juju/romulus/cmd/listplans"
	"github.com/juju/romulus/cmd/setbudget"
	"github.com/juju/romulus/cmd/setplan"
	"github.com/juju/romulus/cmd/showbudget"
	"github.com/juju/romulus/cmd/updateallocation"
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
