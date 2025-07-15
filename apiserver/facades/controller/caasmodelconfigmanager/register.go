// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASModelConfigManager", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newFacade creates a new authorized Facade.
func newFacade(ctx facade.ModelContext) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	domainServices := ctx.DomainServices()
	return &Facade{
		auth:            authorizer,
		watcherRegistry: ctx.WatcherRegistry(),
		controllerConfigAPI: common.NewControllerConfigAPI(
			domainServices.ControllerConfig(),
			domainServices.ControllerNode(),
			domainServices.ExternalController(),
			domainServices.Model(),
		),
		controllerConfigService: domainServices.ControllerConfig(),
	}, nil
}
