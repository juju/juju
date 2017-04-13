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
	"github.com/juju/juju/cmd/juju/romulus/sla"
)

type commandRegister interface {
	Register(cmd.Command)
}

// RegisterAll registers all romulus commands with the
// provided command registry.
func RegisterAll(r commandRegister) {
	r.Register(agree.NewAgreeCommand())
	r.Register(listagreements.NewListAgreementsCommand())
	r.Register(allocate.NewAllocateCommand())
	r.Register(listbudgets.NewListBudgetsCommand())
	r.Register(createbudget.NewCreateBudgetCommand())
	r.Register(listplans.NewListPlansCommand())
	r.Register(setbudget.NewSetBudgetCommand())
	r.Register(setplan.NewSetPlanCommand())
	r.Register(showbudget.NewShowBudgetCommand())
	r.Register(sla.NewSLACommand())
}
