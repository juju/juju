// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/names"
)

type Backend interface {
	ModelUUID() string
	MigrationPhase() (migration.Phase, error)
	WatchMigrationPhase() (state.NotifyWatcher, error)
}

type Facade struct {
	backend   Backend
	resources *common.Resources
}

func New(backend Backend, resources *common.Resources, auth common.Authorizer) (*Facade, error) {
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &Facade{
		backend:   backend,
		resources: resources,
	}, nil
}

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

func (facade *Facade) oneWatch(tagString string) (string, error) {
	if err := facade.auth(tagString); err != nil {
		return "", errors.Trace(err)
	}
	watch, err := facade.backend.WatchMigrationPhase()
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := <-watch.Changes(); ok {
		return facade.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}
