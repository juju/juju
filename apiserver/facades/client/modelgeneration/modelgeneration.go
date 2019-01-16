// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	//"github.com/juju/juju/core/model"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// ModelGeneration defines the methods exported by the model generation API facade.
type ModelGeneration interface {
}

// ModelGenerationState represents the state of an model required by the ModelGeneration.
//go:generate mockgen -package mocks -destination mocks/modelgeneration_mock.go github.com/juju/juju/apiserver/facades/agent/modelgeneration ModelGenerationState
type ModelGenerationState interface {

	// Application returns a application state by name.
	Application(name string) (*state.Application, error)

	// NextGeneration returns the model's "next" generation
	// if one exists that is not yet completed.
	NextGeneration() (*state.Generation, error)
}

type Generation interface {
	AssignUnit(string) error
	CanAutoComplete() (bool, error)
	MakeCurrent() error
}

// ModelGenerationAPI implements the ModelGeneration interface and is the concrete implementation
// of the API endpoint.
type ModelGenerationAPI struct {
	state     ModelGenerationState
	resources facade.Resources
}

// NewModelGenerationFacade provides the signature required for facade registration.
func NewModelGenerationFacade(ctx facade.Context) (*ModelGenerationAPI, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewModelGenerationAPI(ctx.State(), resources, authorizer)
}

// NewModelGenerationAPI creates a new API endpoint for dealing with model generations.
// TODO: (hml) 15-01-2019
// are the following 2 interfaces mocked somewhere else?
//go:generate mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Resources,Authorizer
func NewModelGenerationAPI(
	st ModelGenerationState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*ModelGenerationAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	return &ModelGenerationAPI{
		state:     st,
		resources: resources,
	}, nil
}

// AdvanceGeneration, adds the provided unit(s) and/or application(s) to
// the "next" generation.  If the generation can auto complete, it is
// made the "current" generation.
func (m *ModelGenerationAPI) AdvanceGeneration(args params.Entities) (params.ErrorResults, error) {
	generation, err := m.state.NextGeneration()
	if err != nil {
		return params.ErrorResults{}, err
	}
	if !generation.Active() {
		return params.ErrorResults{}, errors.Errorf("next generation is not active")
	}
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		switch tag.Kind() {
		case names.ApplicationTagKind:
			results.Results[i].Error = common.ServerError(generation.AssignAllUnits(tag.Id()))
		case names.UnitTagKind:
			results.Results[i].Error = common.ServerError(generation.AssignUnit(tag.String()))
		default:
			results.Results[i].Error = common.ServerError(errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag))
		}
	}
	ok, err := generation.CanAutoComplete()
	if err != nil {
		return results, err
	}
	if ok {
		return results, generation.MakeCurrent()
	}

	return results, nil
}
