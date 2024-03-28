// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/leadership"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretsprovider "github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// SecretsDrainAPI is the implementation for the SecretsDrain facade.
type SecretsDrainAPI struct {
	authTag           names.Tag
	logger            loggo.Logger
	leadershipChecker leadership.Checker
	watcherRegistry   facade.WatcherRegistry

	model         Model
	secretService SecretService
}

// NewSecretsDrainAPI returns a new SecretsDrainAPI.
func NewSecretsDrainAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
	logger loggo.Logger,
	leadershipChecker leadership.Checker,
	model Model,
	secretService SecretService,
	watcherRegistry facade.WatcherRegistry,
) (*SecretsDrainAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &SecretsDrainAPI{
		authTag:           authTag,
		logger:            logger,
		leadershipChecker: leadershipChecker,
		model:             model,
		secretService:     secretService,
		watcherRegistry:   watcherRegistry,
	}, nil
}

func filterSecret(controllerUUID, activeBackend string, md *coresecrets.SecretMetadata, rev *coresecrets.SecretRevisionMetadata) bool {
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
}

// GetSecretsToDrain returns metadata for the secrets that need to be drained.
func (s *SecretsDrainAPI) GetSecretsToDrain(ctx context.Context) (params.ListSecretResults, error) {
	modelConfig, err := s.model.ModelConfig(ctx)
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

	var (
		metadata         []*coresecrets.SecretMetadata
		revisionMetadata [][]*coresecrets.SecretRevisionMetadata
	)
	if s.authTag.Kind() == names.ModelTagKind {
		metadata, revisionMetadata, err = s.secretService.ListUserSecrets(ctx)
	} else {
		metadata, revisionMetadata, err = s.getCharmSecrets(ctx)
	}
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
	}

	var result params.ListSecretResults
	for i, md := range metadata {
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

		for _, r := range revisionMetadata[i] {
			if !filterSecret(controllerUUID, activeBackend, md, r) {
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

// isLeaderUnit returns true if the authenticated caller is the unit leader of its application.
func isLeaderUnit(authTag names.Tag, leadershipChecker leadership.Checker) (bool, error) {
	_, err := LeadershipToken(authTag, leadershipChecker)
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

func (s *SecretsDrainAPI) getCharmSecrets(ctx context.Context) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	ownerName := s.authTag.Id()
	owner := secretservice.CharmSecretOwners{
		UnitName: &ownerName,
	}
	// Unit leaders can also get metadata for secrets owned by the app.
	isLeader, err := isLeaderUnit(s.authTag, s.leadershipChecker)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if isLeader {
		appName := AuthTagApp(s.authTag)
		owner.ApplicationName = &appName
	}
	return s.secretService.ListCharmSecrets(ctx, owner)
}

// ChangeSecretBackend updates the backend for the specified secret after migration done.
func (s *SecretsDrainAPI) ChangeSecretBackend(ctx context.Context, args params.ChangeSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.changeSecretBackendForOne(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsDrainAPI) changeSecretBackendForOne(ctx context.Context, arg params.ChangeSecretBackendArg) (err error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	token, err := CanManage(ctx, s.secretService, s.leadershipChecker, s.authTag, uri)
	if err != nil {
		return errors.Trace(err)
	}
	return s.secretService.ChangeSecretBackend(ctx, uri, arg.Revision, toChangeSecretBackendParams(token, arg))
}

func toChangeSecretBackendParams(token leadership.Token, arg params.ChangeSecretBackendArg) secretservice.ChangeSecretBackendParams {
	params := secretservice.ChangeSecretBackendParams{
		LeaderToken: token,
		Data:        arg.Content.Data,
	}
	if arg.Content.ValueRef != nil {
		params.ValueRef = &coresecrets.ValueRef{
			BackendID:  arg.Content.ValueRef.BackendID,
			RevisionID: arg.Content.ValueRef.RevisionID,
		}
	}
	return params
}

// WatchSecretBackendChanged sets up a watcher to notify of changes to the secret backend.
func (s *SecretsDrainAPI) WatchSecretBackendChanged(ctx context.Context) (params.NotifyWatchResult, error) {
	stateWatcher := s.model.WatchForModelConfigChanges()
	w, err := newSecretBackendModelConfigWatcher(ctx, s.model, stateWatcher, s.logger)
	if err != nil {
		return params.NotifyWatchResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}
	id, _, err := internal.EnsureRegisterWatcher[struct{}](ctx, s.watcherRegistry, w)
	if err != nil {
		return params.NotifyWatchResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}
	return params.NotifyWatchResult{
		NotifyWatcherId: id,
	}, nil
}
