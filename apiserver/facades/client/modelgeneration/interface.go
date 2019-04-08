// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
)

//go:generate mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/facades/client/modelgeneration State,Model,Generation,Application

// State represents the state of a model required by the model generation API.
type State interface {
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	Application(string) (Application, error)
}

// Model describes model state used by the model generation API.
type Model interface {
	AddBranch(string, string) error
	Branch(string) (Generation, error)
}

// Generation defines the methods used by a generation.
type Generation interface {
	BranchName() string
	Created() int64
	CreatedBy() string
	AssignAllUnits(string) error
	AssignUnit(string) error
	AssignedUnits() map[string][]string
	Commit(string) (int, error)
	Refresh() error
}

// Application describes application state used by the model generation API.
type Application interface {
	CharmConfig(string) (charm.Settings, error)
}
