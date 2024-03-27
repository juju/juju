// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
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
func (s *UserSecretsManager) WatchRevisionsToPrune(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	w, err := s.secretService.WatchUserSecretsRevisionsToPrune(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	if changes, ok := <-w.Changes(); ok {
		result.StringsWatcherId = s.resources.Register(w)
		result.Changes = changes
	}
	return result, nil
}

// DeleteRevisions deletes the specified revisions of the specified secret.
func (s *UserSecretsManager) DeleteRevisions(ctx context.Context, args params.DeleteSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	if len(args.Args) == 0 {
		return result, nil
	}

	canDelete := func(uri *coresecrets.URI) error {
		md, err := s.secretService.GetSecret(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		if !md.AutoPrune {
			return errors.Errorf("cannot delete non auto-prune secret %q", uri.String())
		}
		return nil
	}

	for i, arg := range args.Args {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = s.secretService.DeleteUserSecret(ctx, uri, arg.Revisions, canDelete)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}
