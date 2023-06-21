// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// srvRelationStatusWatcher defines the API wrapping a state.RelationStatusWatcher.
type srvRelationStatusWatcher struct {
	watcherCommon
	st      *state.State
	watcher state.StringsWatcher
}

func NewRelationStatusWatcher(context facade.Context) (facade.Facade, error) {
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
	stringsWatcher, ok := watcher.(state.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRelationStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       stringsWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvRelationStatusWatcher.
func (w *srvRelationStatusWatcher) Next() (params.RelationLifeSuspendedStatusWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		changesParams := make([]params.RelationLifeSuspendedStatusChange, len(changes))
		for i, key := range changes {
			change, err := crossmodel.GetRelationLifeSuspendedStatusChange(crossmodel.GetBackend(w.st), key)
			if err != nil {
				return params.RelationLifeSuspendedStatusWatchResult{
					Error: apiservererrors.ServerError(err),
				}, nil
			}
			changesParams[i] = *change
		}
		return params.RelationLifeSuspendedStatusWatchResult{
			Changes: changesParams,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RelationLifeSuspendedStatusWatchResult{}, err
}
