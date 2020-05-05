// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/state/storage"
)

// rawState is a wrapper around state.State that supports the needs
// of resources.
type rawState struct {
	base    *State
	persist Persistence
}

// NewResourceState is a function that may be passed to
// state.SetResourcesComponent().
func NewResourceState(persist Persistence, base *State) Resources {
	return &resourceState{
		persist: NewResourcePersistence(persist),
		raw: rawState{
			base:    base,
			persist: persist,
		},
		storage:               persist.NewStorage(),
		dockerMetadataStorage: NewDockerMetadataStorage(base),
		clock:                 base.clock(),
	}
}

// Persistence implements resource/state.RawState.
func (st rawState) Persistence() Persistence {
	return st.persist
}

// Storage implements resource/state.RawState.
func (st rawState) Storage() storage.Storage {
	return st.persist.NewStorage()
}

// Units returns the tags for all units in the application.
func (st rawState) Units(applicationID string) (tags []names.UnitTag, err error) {
	app, err := st.base.Application(applicationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := app.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, u := range units {
		tags = append(tags, u.UnitTag())
	}
	return tags, nil
}

// VerifyApplication implements resource/state.RawState.
func (st rawState) VerifyApplication(id string) error {
	app, err := st.base.Application(id)
	if err != nil {
		return errors.Trace(err)
	}
	if app.Life() != Alive {
		return errors.NewNotFound(nil, fmt.Sprintf("application %q dying or dead", id))
	}
	return nil
}
