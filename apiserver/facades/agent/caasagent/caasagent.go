// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources
	state     CAASAgentState
	cloudspec.CloudSpecAPI

	model Model
}

// NewStateFacade provides the signature required for facade registration.
func NewStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpecAPI := cloudspec.NewCloudSpec(
		cloudspec.MakeCloudSpecGetterForModel(ctx.State()),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return NewFacade(resources, authorizer, stateShim{ctx.State()}, cloudSpecAPI, model)
}

// NewFacade returns a new CAASAgent facade.
func NewFacade(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASAgentState,
	cloudSpecAPI cloudspec.CloudSpecAPI,
	model Model,
) (*Facade, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &Facade{
		CloudSpecAPI: cloudSpecAPI,
		auth:         authorizer,
		resources:    resources,
		state:        st,
		model:        model,
	}, nil
}

// Model returns the details about the model.
func (f *Facade) Model() (params.Model, error) {
	return params.Model{
		Name:     f.model.Name(),
		Type:     string(f.model.Type()),
		UUID:     f.model.UUID(),
		OwnerTag: f.model.Owner().String(),
	}, nil
}
