// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
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
