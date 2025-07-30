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

	leadershipRevoker, err := ctx.LeadershipRevoker()
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}
	watcherRegistry := ctx.WatcherRegistry()

	domainServices := ctx.DomainServices()

	return NewDeployerAPI(
		domainServices.AgentPassword(),
		domainServices.ControllerConfig(),
		domainServices.Application(),
		domainServices.ControllerNode(),
		domainServices.Status(),
		domainServices.Removal(),
		authorizer,
		ctx.ObjectStore(),
		leadershipRevoker,
		watcherRegistry,
		ctx.Clock(),
	)
}
