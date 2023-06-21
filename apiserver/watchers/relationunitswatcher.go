// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// srvRelationUnitsWatcher defines the API wrapping a state.RelationUnitsWatcher.
// It notifies about units entering and leaving the scope of a RelationUnit,
// and changes to the settings of those units known to have entered.
type srvRelationUnitsWatcher struct {
	watcherCommon
	watcher common.RelationUnitsWatcher
}

func NewRelationUnitsWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
		resources       = context.Resources()
	)

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, err := GetWatcherByID(watcherRegistry, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	relationUnitsWatcher, ok := watcher.(common.RelationUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationUnitsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       relationUnitsWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationUnitsWatcher.
func (w *srvRelationUnitsWatcher) Next() (params.RelationUnitsWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.RelationUnitsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RelationUnitsWatchResult{}, err
}
