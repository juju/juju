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

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("CAASAgent", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacadeV1(ctx)
	}, reflect.TypeOf((*FacadeV1)(nil)))
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
	return &FacadeV2{
		CloudSpecer:         cloudSpecAPI,
		ModelWatcher:        common.NewModelWatcher(model, resources, authorizer),
		ControllerConfigAPI: common.NewStateControllerConfig(ctx.State()),
		auth:                authorizer,
		resources:           resources,
	}, nil
}

// newStateFacadeV1 provides the signature required for facade registration of
// caas agent v1
func newStateFacadeV1(ctx facade.Context) (*FacadeV1, error) {
	v2Facade, err := newStateFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	v2Facade.CloudSpecer = cloudspec.NewCloudSpecV1(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(ctx.State()),
		cloudspec.MakeCloudSpecWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(ctx.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(ctx.State()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return &FacadeV1{
		v2Facade,
	}, nil
}
