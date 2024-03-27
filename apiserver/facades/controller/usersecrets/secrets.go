// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// UserSecretsManager is the implementation for the usersecrets facade.
type UserSecretsManager struct {
	authorizer facade.Authorizer
	resources  facade.Resources

	secretService SecretService
}

// WatchRevisionsToPrune returns a watcher for notifying when:
//   - a secret revision owned by the model no longer
//     has any consumers and should be pruned.
func (s *UserSecretsManager) WatchRevisionsToPrune(ctx context.Context) (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w, err := s.secretService.WatchObsoleteUserSecrets(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	if _, ok := <-w.Changes(); ok {
		result.NotifyWatcherId = s.resources.Register(w)
	}
	return result, nil
}

// DeleteObsoleteUserSecrets deletes any obsolete user secret revisions.
func (s *UserSecretsManager) DeleteObsoleteUserSecrets(ctx context.Context) error {
	return s.secretService.DeleteObsoleteUserSecrets(ctx)
}
