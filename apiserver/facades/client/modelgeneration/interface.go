// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

// ModelGeneration defines the methods exported by the model generation API facade.
type ModelGeneration interface {
	AddGeneration() (params.ErrorResult, error)
	AdvanceGeneration(args params.Entities) (params.ErrorResults, error)
	SwitchGeneration(arg params.GenerationVersionArg) (params.ErrorResult, error)
}

// ModelGenerationState represents the state of an model required by the ModelGeneration.
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration ModelGenerationState
type ModelGenerationState interface {
	ControllerTag() names.ControllerTag
	Model() (GenerationModel, error)
}

//go:generate mockgen -package mocks -destination mocks/model_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration GenerationModel
type GenerationModel interface {
	AddGeneration() error
	NextGeneration() (Generation, error)
	SwitchGeneration(version model.GenerationVersion) error
}

// Generation defines the methods used by a generation.
//go:generate mockgen -package mocks -destination mocks/generation_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration Generation
type Generation interface {
	Active() bool
	AssignAllUnits(string) error
	AssignUnit(string) error
	CanMakeCurrent() (bool, error)
	MakeCurrent() error
	Refresh() error
}
