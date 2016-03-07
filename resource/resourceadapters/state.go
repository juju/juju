// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
	corestate "github.com/juju/juju/state"
)

type service struct {
	*corestate.Service
}

// ID returns the service's tag.
func (s *service) ID() names.ServiceTag {
	return names.NewServiceTag(s.Name())
}

// CharmURL implements resource/workers.Service.
func (s *service) CharmURL() *charm.URL {
	cURL, _ := s.Service.CharmURL()
	return cURL
}

// DataStore implements functionality wrapping state for resources.
type DataStore struct {
	corestate.Resources
	State *corestate.State
}

// Units returns the tags for all units in the service.
func (d DataStore) Units(serviceID string) (tags []names.UnitTag, err error) {
	svc, err := d.State.Service(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := svc.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, u := range units {
		tags = append(tags, u.UnitTag())
	}
	return tags, nil
}

// RawState is a wrapper around state.State that supports the needs
// of resources.
type RawState struct {
	// Persist is the persistence layer underlying state.
	Persist corestate.Persistence
}

// Persistence implements resource/state.RawState.
func (st RawState) Persistence() state.Persistence {
	persist := corestate.NewResourcePersistence(st.Persist)
	return resourcePersistence{persist}
}

// Storage implements resource/state.RawState.
func (st RawState) Storage() state.Storage {
	return st.Persist.NewStorage()
}

type resourcePersistence struct {
	*corestate.ResourcePersistence
}

// StageResource implements state.resourcePersistence.
func (p resourcePersistence) StageResource(res resource.Resource, storagePath string) (state.StagedResource, error) {
	return p.ResourcePersistence.StageResource(res, storagePath)
}
