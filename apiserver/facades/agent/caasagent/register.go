// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

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
	}, reflect.TypeFor[*FacadeV2]())
	registry.MustRegister("CAASAgent", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewFacadeV3AuthCheck(ctx)
	}, reflect.TypeFor[*FacadeV3]())
}

// NewFacadeV2AuthCheck provides the signature required for facade registration of
// caas agent v2.
func NewFacadeV2AuthCheck(ctx facade.ModelContext) (*FacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return newFacadeV2(ctx), nil
}

func newFacadeV2(ctx facade.ModelContext) *FacadeV2 {
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
			domainServices.ControllerConfig(),
			domainServices.ControllerNode(),
			domainServices.ExternalController(),
			domainServices.Model(),
		),
		domainServices.ModelProvider(),
		modelCredentialWatcher,
	)
}

// NewFacadeV3AuthCheck provides the signature required for facade registration
// of the controller-only CAAS agent v3.
func NewFacadeV3AuthCheck(ctx facade.ModelContext) (*FacadeV3, error) {
	authorizer := ctx.Auth()
	_, isControllerAgent := authorizer.GetAuthTag().(names.ControllerAgentTag)
	if !authorizer.AuthModelAgent() && !isControllerAgent {
		return nil, apiservererrors.ErrPerm
	}
	return &FacadeV3{
		FacadeV2: newFacadeV2(ctx),
		APIAddresser: common.NewAPIAddresser(
			ctx.DomainServices().ControllerNode(), ctx.WatcherRegistry(),
		),
	}, nil
}
