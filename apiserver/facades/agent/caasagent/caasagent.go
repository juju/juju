// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
)

type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	cloudspec.CloudSpecAPI
	*common.ModelWatcher
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	resources := ctx.Resources()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpecAPI := cloudspec.NewCloudSpec(
		cloudspec.MakeCloudSpecGetterForModel(ctx.State()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return &Facade{
		CloudSpecAPI: cloudSpecAPI,
		ModelWatcher: common.NewModelWatcher(model, resources, authorizer),
		auth:         authorizer,
		resources:    resources,
	}, nil
}
