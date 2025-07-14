// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/watcher"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAgent", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewFacadeV2AuthCheck(ctx)
	}, reflect.TypeOf((*FacadeV2)(nil)))
}

// NewFacadeV2AuthCheck provides the signature required for facade registration of
// caas agent v2.
func NewFacadeV2AuthCheck(ctx facade.ModelContext) (*FacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	modelService := ctx.DomainServices().Model()
	modelCredentialWatcher := func(stdCtx context.Context) (watcher.NotifyWatcher, error) {
		return modelService.WatchModelCloudCredential(stdCtx, ctx.ModelUUID())
	}

	domainServices := ctx.DomainServices()
	registry := ctx.WatcherRegistry()
	return NewFacadeV2(
		ctx.ModelUUID(),
		registry,
		commonmodel.NewModelConfigWatcher(
			domainServices.Config(), registry,
		),
		common.NewControllerConfigAPI(
			ctx.State(),
			domainServices.ControllerConfig(),
			domainServices.ControllerNode(),
			domainServices.ExternalController(),
			domainServices.Model(),
		),
		domainServices.ModelProvider(),
		modelCredentialWatcher,
	), nil
}
