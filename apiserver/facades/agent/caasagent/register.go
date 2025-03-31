// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAgent", 2, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacadeV2(ctx)
	}, reflect.TypeOf((*FacadeV2)(nil)))
}

// newStateFacadeV2 provides the signature required for facade registration of
// caas agent v2
func newStateFacadeV2(ctx facade.ModelContext) (*FacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() && !authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	resources := ctx.Resources()

	authFunc := common.AuthFuncForTag(names.NewModelTag(ctx.ModelUUID().String()))

	domainServices := ctx.DomainServices()

	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		resources,
		cloudspec.MakeCloudSpecGetterForModel(ctx.State(), domainServices.Cloud(), domainServices.Credential(), domainServices.Config()),
		cloudspec.MakeCloudSpecWatcherForModel(ctx.State(), domainServices.Cloud()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(ctx.State(), domainServices.Credential()),
		authFunc,
	)

	return &FacadeV2{
		CloudSpecer:        cloudSpecAPI,
		ModelConfigWatcher: commonmodel.NewModelConfigWatcher(domainServices.Config(), ctx.WatcherRegistry()),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			ctx.State(),
			domainServices.ControllerConfig(),
			domainServices.ExternalController(),
		),
	}, nil
}
