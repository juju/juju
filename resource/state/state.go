// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.resource.state")

// Persistence is the state persistence functionality needed for resources.
type Persistence interface {
	resourcePersistence
}

// Storage is the state storage functionality needed for resources.
type Storage interface {
	resourceStorage
}

// RawState defines the functionality needed from state.State for resources.
type RawState interface {
	// Persistence exposes the state data persistence needed for resources.
	Persistence() Persistence

	// Storage exposes the state blob storage needed for resources.
	Storage() Storage
}

// State exposes the state functionality needed for resources.
type State struct {
	*resourceState
}

// NewState returns a new State for the given raw Juju state.
func NewState(raw RawState) *State {
	logger.Tracef("wrapping state for resources")

	persist := raw.Persistence()

	storage := raw.Storage()

	st := &State{
		resourceState: &resourceState{
			persist:      persist,
			storage:      storage,
			newPendingID: newPendingID,
			currentTimestamp: func() time.Time {
				return time.Now().UTC()
			},
		},
	}
	return st
}
