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

// srvSecretTriggerWatcher defines the API wrapping a SecretsTriggerWatcher.
type srvSecretTriggerWatcher struct {
	watcherCommon
	watcher state.SecretsTriggerWatcher
}

func NewSecretsTriggerWatcher(context facade.Context) (facade.Facade, error) {
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
	secretsTriggerWatcher, ok := watcher.(state.SecretsTriggerWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretTriggerWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       secretsTriggerWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretTriggerWatcher) Next() (params.SecretTriggerWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		return params.SecretTriggerWatchResult{
			Changes: w.translateChanges(changes),
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.SecretTriggerWatchResult{}, err
}

func (w *srvSecretTriggerWatcher) translateChanges(changes []corewatcher.SecretTriggerChange) []params.SecretTriggerChange {
	if changes == nil {
		return nil
	}
	result := make([]params.SecretTriggerChange, len(changes))
	for i, c := range changes {
		result[i] = params.SecretTriggerChange{
			URI:             c.URI.String(),
			Revision:        c.Revision,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	return result
}
