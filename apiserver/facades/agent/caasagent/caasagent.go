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
