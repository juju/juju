// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// srvSecretsRevisionWatcher defines the API wrapping a SecretsRevisionWatcher.
type srvSecretsRevisionWatcher struct {
	watcherCommon
	st      *state.State
	watcher state.StringsWatcher
}

func NewSecretsRevisionWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
	)

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}
	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	stringsWatcher, ok := watcher.(state.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvSecretsRevisionWatcher{
		watcherCommon: newWatcherCommon(context),
		st:            context.State(),
		watcher:       stringsWatcher,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the srvSecretRotationWatcher.
func (w *srvSecretsRevisionWatcher) Next() (params.SecretRevisionWatchResult, error) {
	if changes, ok := <-w.watcher.Changes(); ok {
		ch, err := w.translateChanges(changes)
		if err != nil {
			return params.SecretRevisionWatchResult{}, errors.Trace(err)
		}
		return params.SecretRevisionWatchResult{
			Changes: ch,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.SecretRevisionWatchResult{}, err
}

func (w *srvSecretsRevisionWatcher) translateChanges(changes []string) ([]params.SecretRevisionChange, error) {
	if changes == nil {
		return nil, nil
	}
	secrets := state.NewSecrets(w.st)
	result := make([]params.SecretRevisionChange, len(changes))
	for i, uriStr := range changes {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		md, err := secrets.GetSecret(uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i] = params.SecretRevisionChange{
			URI:      uri.String(),
			Revision: md.LatestRevision,
		}
	}
	return result, nil
}
