// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// FacadeV2 is the V2 facade of the caas agent
type FacadeV2 struct {
	auth      facade.Authorizer
	resources facade.Resources
	cloudspec.CloudSpecer
	*common.ModelWatcher
	*common.ControllerConfigAPI
}

// FacadeV1 is the V1 facade of the caas agent
type FacadeV1 struct {
	*FacadeV2
}

// NewStateFacadeV2 provides the signature required for facade registration of
// caas agent v2
func NewStateFacadeV2(ctx facade.Context) (*FacadeV2, error) {
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

// NewStateFacadeV1 provides the signature required for facade registration of
// caas agent v1
func NewStateFacadeV1(ctx facade.Context) (*FacadeV1, error) {
	v2Facade, err := NewStateFacadeV2(ctx)
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
