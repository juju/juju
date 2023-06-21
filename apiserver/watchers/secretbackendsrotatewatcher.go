// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// srvSecretBackendsRotateWatcher defines the API wrapping a SecretBackendsRotateWatcher.
type srvSecretBackendsRotateWatcher struct {
	watcherCommon
	watcher state.SecretBackendRotateWatcher
}

func NewSecretBackendsRotateWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
	)

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendRotateWatcher, ok := watcher.(state.SecretBackendRotateWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretBackendsRotateWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       secretBackendRotateWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretBackendsRotateWatcher) Next() (params.SecretBackendRotateWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.SecretBackendRotateWatchResult{
			Changes: w.translateChanges(changes),
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.SecretBackendRotateWatchResult{}, err
}

func (w *srvSecretBackendsRotateWatcher) translateChanges(changes []corewatcher.SecretBackendRotateChange) []params.SecretBackendRotateChange {
	if changes == nil {
		return nil
	}
	result := make([]params.SecretBackendRotateChange, len(changes))
	for i, c := range changes {
		result[i] = params.SecretBackendRotateChange{
			ID:              c.ID,
			Name:            c.Name,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	return result
}
