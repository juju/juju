// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASAgent", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacadeV2(ctx)
	}, reflect.TypeOf((*FacadeV2)(nil)))
}

// newStateFacadeV2 provides the signature required for facade registration of
// caas agent v2
func newStateFacadeV2(ctx facade.Context) (*FacadeV2, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() && !authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	resources := ctx.Resources()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		resources,
		cloudspec.MakeCloudSpecGetterForModel(ctx.State()),
		cloudspec.MakeCloudSpecWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(ctx.State()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	serviceFactory := ctx.ServiceFactory()
	return &FacadeV2{
		CloudSpecer:  cloudSpecAPI,
		ModelWatcher: common.NewModelWatcher(model, resources, authorizer),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			ctx.State(),
			serviceFactory.ControllerConfig(),
			serviceFactory.ExternalController(),
		),
	}, nil
}
