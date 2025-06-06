// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Deployer", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewDeployerFacade(ctx)
	}, reflect.TypeOf((*DeployerAPI)(nil)))
}

// NewDeployerFacade creates a new server-side DeployerAPI facade.
func NewDeployerFacade(ctx facade.ModelContext) (*DeployerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	leadershipRevoker, err := ctx.LeadershipRevoker()
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}
	watcherRegistry := ctx.WatcherRegistry()

	domainServices := ctx.DomainServices()

	controllerConfigGetter := domainServices.ControllerConfig()
	controllerNodeService := domainServices.ControllerNode()
	applicationService := domainServices.Application()
	statusService := domainServices.Status()

	return NewDeployerAPI(
		domainServices.AgentPassword(),
		controllerConfigGetter,
		applicationService,
		controllerNodeService,
		statusService,
		authorizer,
		st,
		ctx.ObjectStore(),
		leadershipRevoker,
		watcherRegistry,
		ctx.Clock(),
	)
}
