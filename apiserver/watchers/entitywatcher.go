// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// EntitiesWatcher defines an interface based on the StringsWatcher
// but also providing a method for the mapping of the received
// strings to the tags of the according entities.
type EntitiesWatcher interface {
	state.StringsWatcher

	// MapChanges maps the received strings to their according tag strings.
	// The EntityFinder interface representing state or a mock has to be
	// upcasted into the needed sub-interface of state for the real mapping.
	MapChanges(in []string) ([]string, error)
}

// srvEntitiesWatcher defines the API for methods on a state.StringsWatcher.
// Each client has its own current set of watchers, stored in resources.
// srvEntitiesWatcher notifies about changes for all entities of a given kind,
// sending the changes as a list of strings, which could be transformed
// from state entity ids to their corresponding entity tags.
type srvEntitiesWatcher struct {
	watcherCommon
	watcher EntitiesWatcher
}

func NewEntitiesWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
		resources       = context.Resources()
	)

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, err := GetWatcherByID(watcherRegistry, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	entitiesWatcher, ok := watcher.(EntitiesWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvEntitiesWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       entitiesWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvEntitiesWatcher.
func (w *srvEntitiesWatcher) Next() (params.EntitiesWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		mapped, err := w.watcher.MapChanges(changes)
		if err != nil {
			return params.EntitiesWatchResult{}, errors.Annotate(err, "cannot map changes")
		}
		return params.EntitiesWatchResult{
			Changes: mapped,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.EntitiesWatchResult{}, err
}
