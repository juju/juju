// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// Backend exposes information about any current model migrations.
type Backend interface {
	ModelUUID() string
	MigrationPhase() (migration.Phase, error)
	WatchMigrationPhase() state.NotifyWatcher
}

// Facade lets clients watch and get models' migration phases.
type Facade struct {
	backend   Backend
	resources facade.Resources
}

// New creates a Facade backed by backend and resources. If auth
// doesn't identity the client as a machine agent or a unit agent,
// it will return common.ErrPerm.
func New(backend Backend, resources facade.Resources, auth facade.Authorizer) (*Facade, error) {
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() && !auth.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	return &Facade{
		backend:   backend,
		resources: resources,
	}, nil
}

// auth is very simplistic: it only accepts the model tag reported by
// the backend.
func (facade *Facade) auth(tagString string) error {
	tag, err := names.ParseModelTag(tagString)
	if err != nil {
		return errors.Trace(err)
	}
	if tag.Id() != facade.backend.ModelUUID() {
		return common.ErrPerm
	}
	return nil
}

// Phase returns the current migration phase or an error for every
// supplied entity.
func (facade *Facade) Phase(entities params.Entities) params.PhaseResults {
	count := len(entities.Entities)
	results := params.PhaseResults{
		Results: make([]params.PhaseResult, count),
	}
	for i, entity := range entities.Entities {
		phase, err := facade.onePhase(entity.Tag)
		results.Results[i].Phase = phase
		results.Results[i].Error = common.ServerError(err)
	}
	return results
}

// onePhase does auth and lookup for a single entity.
func (facade *Facade) onePhase(tagString string) (string, error) {
	if err := facade.auth(tagString); err != nil {
		return "", errors.Trace(err)
	}
	phase, err := facade.backend.MigrationPhase()
	if err != nil {
		return "", errors.Trace(err)
	}
	return phase.String(), nil
}

// Watch returns an id for use with the NotifyWatcher facade, or an
// error, for every supplied entity.
func (facade *Facade) Watch(entities params.Entities) params.NotifyWatchResults {
	count := len(entities.Entities)
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, count),
	}
	for i, entity := range entities.Entities {
		id, err := facade.oneWatch(entity.Tag)
		results.Results[i].NotifyWatcherId = id
		results.Results[i].Error = common.ServerError(err)
	}
	return results
}

// oneWatch does auth, and watcher creation/registration, for a single
// entity.
func (facade *Facade) oneWatch(tagString string) (string, error) {
	if err := facade.auth(tagString); err != nil {
		return "", errors.Trace(err)
	}
	watch := facade.backend.WatchMigrationPhase()
	if _, ok := <-watch.Changes(); ok {
		return facade.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}
