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
	AddGeneration(entity params.Entity) (params.ErrorResult, error)
	AdvanceGeneration(params.AdvanceGenerationArg) (params.AdvanceGenerationResult, error)
	SwitchGeneration(params.GenerationVersionArg) (params.ErrorResult, error)
	CancelGeneration(entity params.Entity) (params.ErrorResult, error)
	HasNextGeneration(params.Entity) (params.BoolResult, error)
	GenerationInfo(params.Entity) (params.GenerationResult, error)
}

// State represents the state of a model required by the model generation API.
type State interface {
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	Application(string) (Application, error)
}

// Model describes model state used by the model generation API.
type Model interface {
	AddGeneration(string) error
	NextGeneration() (Generation, error)
	HasNextGeneration() (bool, error)
}

// Generation defines the methods used by a generation.
type Generation interface {
	Created() int64
	CreatedBy() string
	AssignAllUnits(string) error
	AssignUnit(string) error
	AssignedUnits() map[string][]string
	MakeCurrent(string) error
	AutoComplete(string) (bool, error)
	Refresh() error
}

// Application describes application state used by the model generation API.
type Application interface {
	CharmConfig(model.GenerationVersion) (charm.Settings, error)
}
