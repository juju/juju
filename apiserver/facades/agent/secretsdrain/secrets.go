// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.secretsdrain")

// For testing.
var (
	GetProvider = secretsprovider.Provider
)

// SecretsDrainAPI is the implementation for the SecretsDrain facade.
type SecretsDrainAPI struct {
	leadershipChecker leadership.Checker
	secretsState      SecretsState
	resources         facade.Resources
	secretsConsumer   SecretsConsumer
	authTag           names.Tag

	model Model
}

func (s *SecretsDrainAPI) getSecretMetadata(
	filter func(*coresecrets.SecretMetadata, *coresecrets.SecretRevisionMetadata) bool,
) (params.ListSecretResults, error) {
	var result params.ListSecretResults
	listFilter := state.SecretsFilter{
		// TODO: there is a bug that operator agents can't get any unit owned secrets!
		// Because the authTag here is the application tag, but not unit tag.
		OwnerTags: []names.Tag{s.authTag},
	}
	// Unit leaders can also get metadata for secrets owned by the app.
	// TODO(wallyworld) - temp fix for old podspec charms
	isLeader, err := s.isLeaderUnit()
	if err != nil {
		return result, errors.Trace(err)
	}
	if isLeader {
		appOwner := names.NewApplicationTag(authTagApp(s.authTag))
		listFilter.OwnerTags = append(listFilter.OwnerTags, appOwner)
	}

	secrets, err := s.secretsState.ListSecrets(listFilter)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, md := range secrets {
		secretResult := params.ListSecretResult{
			URI:              md.URI.String(),
			Version:          md.Version,
			OwnerTag:         md.OwnerTag,
			RotatePolicy:     md.RotatePolicy.String(),
			NextRotateTime:   md.NextRotateTime,
			Description:      md.Description,
			Label:            md.Label,
			LatestRevision:   md.LatestRevision,
			LatestExpireTime: md.LatestExpireTime,
			CreateTime:       md.CreateTime,
			UpdateTime:       md.UpdateTime,
		}
		revs, err := s.secretsState.ListSecretRevisions(md.URI)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		for _, r := range revs {
			if filter != nil && !filter(md, r) {
				continue
			}
			var valueRef *params.SecretValueRef
			if r.ValueRef != nil {
				valueRef = &params.SecretValueRef{
					BackendID:  r.ValueRef.BackendID,
					RevisionID: r.ValueRef.RevisionID,
				}
			}
			secretResult.Revisions = append(secretResult.Revisions, params.SecretRevision{
				Revision: r.Revision,
				ValueRef: valueRef,
			})
		}
		if len(secretResult.Revisions) == 0 {
			continue
		}
		result.Results = append(result.Results, secretResult)
	}
	return result, nil
}

// GetSecretsToDrain returns metadata for the secrets that need to be drained.
func (s *SecretsDrainAPI) GetSecretsToDrain() (params.ListSecretResults, error) {
	modelConfig, err := s.model.ModelConfig()
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
	}
	modelType := s.model.Type()
	modelUUID := s.model.UUID()
	controllerUUID := s.model.ControllerUUID()

	activeBackend := modelConfig.SecretBackend()
	if activeBackend == secretsprovider.Auto {
		activeBackend = controllerUUID
		if modelType == state.ModelTypeCAAS {
			activeBackend = modelUUID
		}
	}
	return s.getSecretMetadata(func(md *coresecrets.SecretMetadata, rev *coresecrets.SecretRevisionMetadata) bool {
		if rev.ValueRef == nil {
			// Only internal backend secrets have nil ValueRef.
			if activeBackend == secretsprovider.Internal {
				return false
			}
			if activeBackend == controllerUUID {
				return false
			}
			return true
		}
		return rev.ValueRef.BackendID != activeBackend
	})
}

// ChangeSecretBackend updates the backend for the specified secret after migration done.
func (s *SecretsDrainAPI) ChangeSecretBackend(args params.ChangeSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.changeSecretBackendForOne(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsDrainAPI) changeSecretBackendForOne(arg params.ChangeSecretBackendArg) (err error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	token, err := s.canManage(uri)
	if err != nil {
		return errors.Trace(err)
	}
	return s.secretsState.ChangeSecretBackend(toChangeSecretBackendParams(token, uri, arg))
}

func toChangeSecretBackendParams(token leadership.Token, uri *coresecrets.URI, arg params.ChangeSecretBackendArg) state.ChangeSecretBackendParams {
	params := state.ChangeSecretBackendParams{
		Token:    token,
		URI:      uri,
		Revision: arg.Revision,
		Data:     arg.Content.Data,
	}
	if arg.Content.ValueRef != nil {
		params.ValueRef = &coresecrets.ValueRef{
			BackendID:  arg.Content.ValueRef.BackendID,
			RevisionID: arg.Content.ValueRef.RevisionID,
		}
	}
	return params
}

func (s *SecretsDrainAPI) isLeaderUnit() (bool, error) {
	if s.authTag.Kind() != names.UnitTagKind {
		return false, nil
	}
	_, err := s.leadershipToken()
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

// WatchSecretBackendChanged sets up a watcher to notify of changes to the secret backend.
func (s *SecretsDrainAPI) WatchSecretBackendChanged() (params.NotifyWatchResult, error) {
	stateWatcher := s.model.WatchForModelConfigChanges()
	w, err := newSecretBackendModelConfigWatcher(s.model, stateWatcher)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}, nil
	}
	if _, ok := <-w.Changes(); ok {
		return params.NotifyWatchResult{NotifyWatcherId: s.resources.Register(w)}, nil
	}
	return params.NotifyWatchResult{Error: apiservererrors.ServerError(watcher.EnsureErr(w))}, nil
}
