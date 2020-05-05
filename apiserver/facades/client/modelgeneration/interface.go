// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/settings"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration State,Model,Generation,Application,ModelCache

// State represents the state of a model required by the model generation API.
type State interface {
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	Application(string) (Application, error)
}

// Model describes model state used by the model generation API.
type Model interface {
	ModelTag() names.ModelTag
	AddBranch(string, string) error
	Branch(string) (Generation, error)
	Branches() ([]Generation, error)
	Generation(int) (Generation, error)
	Generations() ([]Generation, error)
}

// ModelCache describes a cached model used by the model generation API.
type ModelCache interface {
	Branch(string) (cache.Branch, error)
}

// Generation defines the methods used by a generation.
type Generation interface {
	BranchName() string
	Created() int64
	CreatedBy() string
	Completed() int64
	CompletedBy() string
	AssignAllUnits(string) error
	AssignUnits(string, int) error
	AssignUnit(string) error
	AssignedUnits() map[string][]string
	Commit(string) (int, error)
	Abort(string) error
	Config() map[string]settings.ItemChanges
	GenerationId() int
}

// Application describes application state used by the model generation API.
type Application interface {
	UnitNames() ([]string, error)

	// DefaultCharmConfig is the only abstraction in these shims.
	// It saves us having to shim out Charm as well.
	DefaultCharmConfig() (charm.Settings, error)
}
