// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
)

// NewWatcherFunc exists to let us unit test Facade without patching.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// NewFacade returns a Facade backed by the supplied api caller.
func NewFacade(apiCaller base.APICaller, newWatcher NewWatcherFunc) *Facade {
	facadeCaller := base.NewFacadeCaller(apiCaller, "MigrationFlag")
	return &Facade{
		caller:     facadeCaller,
		newWatcher: newWatcher,
	}
}

// Facade lets a client watch and query a model's migration phase.
type Facade struct {
	caller     base.FacadeCaller
	newWatcher NewWatcherFunc
}

// Phase returns the current migration.Phase for the supplied model UUID.
func (facade *Facade) Phase(uuid string) (migration.Phase, error) {
	results := params.PhaseResults{}
	err := facade.call("Phase", uuid, &results)
	if err != nil {
		return migration.UNKNOWN, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return migration.UNKNOWN, countError(count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return migration.UNKNOWN, errors.Trace(result.Error)
	}
	phase, ok := migration.ParsePhase(result.Phase)
	if !ok {
		err := errors.Errorf("unknown phase %q", result.Phase)
		return migration.UNKNOWN, err
	}
	return phase, nil
}

// Watch returns a NotifyWatcher that will inform of potential changes
// to the result of Phase for the supplied model UUID.
func (facade *Facade) Watch(uuid string) (watcher.NotifyWatcher, error) {
	results := params.NotifyWatchResults{}
	err := facade.call("Watch", uuid, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return nil, countError(count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	apiCaller := facade.caller.RawAPICaller()
	watcher := facade.newWatcher(apiCaller, result)
	return watcher, nil
}

// call converts the supplied model uuid into a params.Entities and
// invokes the facade caller.
func (facade *Facade) call(name, uuid string, results interface{}) error {
	model := names.NewModelTag(uuid).String()
	args := params.Entities{[]params.Entity{{model}}}
	err := facade.caller.FacadeCall(name, args, results)
	return errors.Trace(err)
}

// countError complains about malformed results.
func countError(count int) error {
	return errors.Errorf("expected 1 result, got %d", count)
}
