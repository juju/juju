// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// srvRemoteRelationWatcher defines the API wrapping a
// state.RelationUnitsWatcher but serving the events it emits as
// fully-expanded params.RemoteRelationChangeEvents so they can be
// used across model/controller boundaries.
type srvRemoteRelationWatcher struct {
	watcherCommon
	backend crossmodel.Backend
	watcher *crossmodel.WrappedUnitsWatcher
}

func NewRemoteRelationWatcher(context facade.Context) (facade.Facade, error) {
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
	if err != nil {
		return nil, errors.Trace(err)
	}
	remoteRelationWatcher, ok := watcher.(*crossmodel.WrappedUnitsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvRemoteRelationWatcher{
		watcherCommon: newWatcherCommon(context),
		backend:       crossmodel.GetBackend(context.State()),
		watcher:       remoteRelationWatcher,
	}, nil
}

func (w *srvRemoteRelationWatcher) Next() (params.RemoteRelationWatchResult, error) {
	if change, ok := <-w.watcher.Changes(); ok {
		// Expand the change into a cross-model event.
		expanded, err := crossmodel.ExpandChange(
			w.backend,
			w.watcher.RelationToken,
			w.watcher.ApplicationToken,
			change,
		)
		if err != nil {
			return params.RemoteRelationWatchResult{
				Error: apiservererrors.ServerError(err),
			}, nil
		}
		return params.RemoteRelationWatchResult{
			Changes: expanded,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.RemoteRelationWatchResult{}, err
}
