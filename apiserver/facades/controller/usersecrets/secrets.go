// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/rpc/params"
)

// UserSecretsManager is the implementation for the usersecrets facade.
type UserSecretsManager struct {
	watcherRegistry facade.WatcherRegistry
	secretService   SecretService
}

// WatchRevisionsToPrune returns a watcher for notifying when:
//   - a secret revision owned by the model no longer
//     has any consumers and should be pruned.
func (s *UserSecretsManager) WatchRevisionsToPrune(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w, err := s.secretService.WatchObsoleteUserSecretsToPrune(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
	result.Error = apiservererrors.ServerError(err)
	return result, nil
}

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions.
func (s *UserSecretsManager) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	return s.secretService.DeleteObsoleteUserSecretRevisions(ctx)
}
