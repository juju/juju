// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

// ModelChange represents either a new model, or a change
// to an existing model.
type ModelChange struct {
	ModelUUID string
	Name      string
	Life      life.Value
	Owner     string // tag maybe?
	Config    map[string]interface{}
	Status    status.StatusInfo
}

// RemoveModel represents the situation when a model is removed
// from the database.
type RemoveModel struct {
	ModelUUID string
}

// ApplicationChange represents either a new application, or a change
// to an existing application in a model.
type ApplicationChange struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	Life            life.Value
	MinUnits        int
	Constraints     constraints.Value
	Config          map[string]interface{}
	Subordinate     bool
	Status          status.StatusInfo
	WorkloadVersion string
}

// RemoveApplication represents the situation when an application
// is removed from a model in the database.
type RemoveApplication struct {
	ModelUUID string
	Name      string
}
