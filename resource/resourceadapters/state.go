// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/state"
	corestate "github.com/juju/juju/state"
)

type service struct {
	*corestate.Service
}

// ID returns the service's tag.
func (s *service) ID() names.ApplicationTag {
	return names.NewApplicationTag(s.Name())
}

// CharmURL implements resource/workers.Service.
func (s *service) CharmURL() *charm.URL {
	cURL, _ := s.Service.CharmURL()
	return cURL
}

// rawState is a wrapper around state.State that supports the needs
// of resources.
type rawState struct {
	base    *corestate.State
	persist corestate.Persistence
}

// NewResourceState is a function that may be passed to
// state.SetResourcesComponent().
func NewResourceState(persist corestate.Persistence, base *corestate.State) corestate.Resources {
	return state.NewState(&rawState{
		base:    base,
		persist: persist,
	})
}

// Persistence implements resource/state.RawState.
func (st rawState) Persistence() state.Persistence {
	persist := corestate.NewResourcePersistence(st.persist)
	return resourcePersistence{persist}
}

// Storage implements resource/state.RawState.
func (st rawState) Storage() state.Storage {
	return st.persist.NewStorage()
}

// Units returns the tags for all units in the service.
func (st rawState) Units(serviceID string) (tags []names.UnitTag, err error) {
	svc, err := st.base.Service(serviceID)
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

// VerifyService implements resource/state.RawState.
func (st rawState) VerifyService(id string) error {
	svc, err := st.base.Service(id)
	if err != nil {
		return errors.Trace(err)
	}
	if svc.Life() != corestate.Alive {
		return errors.NewNotFound(nil, fmt.Sprintf("service %q dying or dead", id))
	}
	return nil
}

type resourcePersistence struct {
	*corestate.ResourcePersistence
}

// StageResource implements state.resourcePersistence.
func (p resourcePersistence) StageResource(res resource.Resource, storagePath string) (state.StagedResource, error) {
	return p.ResourcePersistence.StageResource(res, storagePath)
}

// NewResourcePersistence is a function that may be passed to
// state.SetResourcesPersistence(). It will be used in the core state
// package to produce the resource persistence.
func NewResourcePersistence(persist corestate.Persistence) corestate.ResourcesPersistence {
	return corestate.NewResourcePersistence(persist)
}

// CleanUpBlob is a cleanup handler that will be used in state cleanup.
func CleanUpBlob(st *corestate.State, persist corestate.Persistence, storagePath string) error {
	// TODO(ericsnow) Move this to state.RemoveResource().
	storage := persist.NewStorage()
	return storage.Remove(storagePath)
}
