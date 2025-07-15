// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/rpc/params"
)

// Facade lets clients watch and get models' migration phases.
type Facade struct {
	watcherRegistry       facade.WatcherRegistry
	getCanAccess          common.GetAuthFunc
	modelMigrationService ModelMigrationService
}

// New creates a Facade backed by backend and resources. If auth
// doesn't identity the client as a machine agent or a unit agent,
// it will return apiservererrors.ErrPerm.
func New(
	watcherRegistry facade.WatcherRegistry,
	auth facade.Authorizer,
	getCanAccess common.GetAuthFunc,
	modelMigrationService ModelMigrationService,
) (*Facade, error) {
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &Facade{
		watcherRegistry:       watcherRegistry,
		getCanAccess:          getCanAccess,
		modelMigrationService: modelMigrationService,
	}, nil
}

// auth is very simplistic: it only accepts the model tag reported by
// the backend.
func (facade *Facade) auth(ctx context.Context, tagString string) error {
	tag, err := names.ParseModelTag(tagString)
	if err != nil {
		return errors.Trace(err)
	}
	canAccess, err := facade.getCanAccess(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if !canAccess(tag) {
		return apiservererrors.ErrPerm
	}
	return nil
}

// Phase returns the current migration phase or an error for every
// supplied entity.
func (facade *Facade) Phase(ctx context.Context, entities params.Entities) params.PhaseResults {
	count := len(entities.Entities)
	results := params.PhaseResults{
		Results: make([]params.PhaseResult, count),
	}
	for i, entity := range entities.Entities {
		phase, err := facade.onePhase(ctx, entity.Tag)
		results.Results[i].Phase = phase
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results
}

// onePhase does auth and lookup for a single entity.
func (facade *Facade) onePhase(ctx context.Context, tagString string) (string, error) {
	if err := facade.auth(ctx, tagString); err != nil {
		return "", errors.Trace(err)
	}
	m, err := facade.modelMigrationService.Migration(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.Phase.String(), nil
}

// Watch returns an id for use with the NotifyWatcher facade, or an
// error, for every supplied entity.
func (facade *Facade) Watch(ctx context.Context, entities params.Entities) params.NotifyWatchResults {
	count := len(entities.Entities)
	results := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, count),
	}
	for i, entity := range entities.Entities {
		id, err := facade.oneWatch(ctx, entity.Tag)
		results.Results[i].NotifyWatcherId = id
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results
}

// oneWatch does auth, and watcher creation/registration, for a single
// entity.
func (facade *Facade) oneWatch(ctx context.Context, tagString string) (string, error) {
	if err := facade.auth(ctx, tagString); err != nil {
		return "", errors.Trace(err)
	}
	w, err := facade.modelMigrationService.WatchMigrationPhase(ctx)
	if err != nil {
		return "", errors.Trace(err)
	}
	id, _, err := internal.EnsureRegisterWatcher(ctx, facade.watcherRegistry, w)
	return id, errors.Trace(err)
}
