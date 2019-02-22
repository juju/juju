// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration APIFacade,State,Model,Generation,Application

// APIFacade defines the methods exported by the model generation API facade.
type APIFacade interface {
	AddGeneration() (params.ErrorResult, error)
	AdvanceGeneration(args params.Entities) (params.ErrorResults, error)
	SwitchGeneration(arg params.GenerationVersionArg) (params.ErrorResult, error)
}

// State represents the state of a model required by the model generation API.
type State interface {
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	Application(string) (Application, error)
}

// Model describes model state used by the model generation API.
type Model interface {
	AddGeneration() error
	NextGeneration() (Generation, error)
	HasNextGeneration() (bool, error)
}

// Generation defines the methods used by a generation.
type Generation interface {
	AssignAllUnits(string) error
	AssignUnit(string) error
	AssignedUnits() map[string][]string
	MakeCurrent() error
	AutoComplete() (bool, error)
	Refresh() error
}

// Application describes application state used by the model generation API.
type Application interface {
	CharmConfig(model.GenerationVersion) (charm.Settings, error)
}
