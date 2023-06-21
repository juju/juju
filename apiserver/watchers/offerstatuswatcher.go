// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelrelations"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// srvOfferStatusWatcher defines the API wrapping a crossmodelrelations.OfferStatusWatcher.
type srvOfferStatusWatcher struct {
	watcherCommon
	st      *state.State
	watcher crossmodelrelations.OfferWatcher
}

func NewOfferStatusWatcher(context facade.Context) (facade.Facade, error) {
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

	offerWatcher, ok := watcher.(crossmodelrelations.OfferWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvOfferStatusWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       offerWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvOfferStatusWatcher.
func (w *srvOfferStatusWatcher) Next() (params.OfferStatusWatchResult, error) {
	if _, ok := <-w.watcher.Changes(); ok {
		change, err := crossmodel.GetOfferStatusChange(
			crossmodel.GetBackend(w.st),
			w.watcher.OfferUUID(), w.watcher.OfferName())
		if err != nil {
			// For the specific case where we are informed that a migration is
			// in progress, we want to return an error that causes the client
			// to stop watching, rather than in the payload.
			if errors.Is(err, migration.ErrMigrating) {
				return params.OfferStatusWatchResult{}, err
			}

			return params.OfferStatusWatchResult{Error: apiservererrors.ServerError(err)}, nil
		}
		return params.OfferStatusWatchResult{
			Changes: []params.OfferStatusChange{*change},
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.OfferStatusWatchResult{}, err
}
