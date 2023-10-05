// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretusersupplied

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state/watcher"
)

// SecretUserSuppliedManager is the implementation for the secretusersupplied facade.
type SecretUserSuppliedManager struct {
	authorizer facade.Authorizer
	resources  facade.Resources

	authTag        names.Tag
	controllerUUID string
	modelUUID      string

	secretsState        SecretsState
	backendConfigGetter func() (*provider.ModelBackendConfigInfo, error)
}

// WatchObsoleteRevisionsNeedPrune returns a watcher for notifying when:
//   - a secret revision owned by the model no longer
//     has any consumers and should be pruned.
func (s *SecretUserSuppliedManager) WatchObsoleteRevisionsNeedPrune() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	w, err := s.secretsState.WatchObsoleteRevisionsNeedPrune([]names.Tag{names.NewModelTag(s.modelUUID)})
	if err != nil {
		return result, errors.Trace(err)
	}
	if changes, ok := <-w.Changes(); ok {
		result.StringsWatcherId = s.resources.Register(w)
		result.Changes = changes
	} else {
		err = watcher.EnsureErr(w)
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// DeleteRevisions deletes the specified revisions of the specified secret.
func (s *SecretUserSuppliedManager) DeleteRevisions(args params.DeleteSecretArgs) (params.ErrorResults, error) {
	return commonsecrets.RemoveSecretsUserSupplied(
		s.secretsState, s.backendConfigGetter,
		s.authTag, args,
		func(uri *coresecrets.URI) error {
			// Only admin can delete user secrets.
			_, err := common.HasModelAdmin(s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
			if err != nil {
				return errors.Trace(err)
			}
			md, err := s.secretsState.GetSecret(uri)
			if err != nil {
				return errors.Trace(err)
			}
			// Can only delete model owned(user supplied) secrets.
			if md.OwnerTag != names.NewModelTag(s.modelUUID).String() {
				return apiservererrors.ErrPerm
			}
			if !md.AutoPrune {
				return errors.Errorf("cannot delete auto-prune secret %q", uri.String())
			}
			return nil
		},
	)
}
