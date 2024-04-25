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
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/rpc/params"
)

// SecretsDrainAPI is the implementation for the SecretsDrain facade.
type SecretsDrainAPI struct {
	authTag           names.Tag
	logger            loggo.Logger
	leadershipChecker leadership.Checker
	watcherRegistry   facade.WatcherRegistry

	model                Model
	secretService        SecretService
	secretBackendService SecretBackendService
}

// NewSecretsDrainAPI returns a new SecretsDrainAPI.
func NewSecretsDrainAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
	logger loggo.Logger,
	leadershipChecker leadership.Checker,
	model Model,
	secretService SecretService,
	secretBackendService SecretBackendService,
	watcherRegistry facade.WatcherRegistry,
) (*SecretsDrainAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &SecretsDrainAPI{
		authTag:              authTag,
		logger:               logger,
		leadershipChecker:    leadershipChecker,
		model:                model,
		secretService:        secretService,
		secretBackendService: secretBackendService,
		watcherRegistry:      watcherRegistry,
	}, nil
}

// GetSecretsToDrain returns metadata for the secrets that need to be drained.
func (s *SecretsDrainAPI) GetSecretsToDrain(ctx context.Context) (params.ListSecretResults, error) {
	var (
		metadata         []*coresecrets.SecretMetadata
		revisionMetadata [][]*coresecrets.SecretRevisionMetadata
		err              error
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
		ownerTag, err := OwnerTagFromOwner(md.Owner)
		if err != nil {
			// This should never happen.
			return params.ListSecretResults{}, errors.Trace(err)
		}
		secretResult := params.ListSecretResult{
			URI:              md.URI.String(),
			Version:          md.Version,
			OwnerTag:         ownerTag.String(),
			RotatePolicy:     md.RotatePolicy.String(),
			NextRotateTime:   md.NextRotateTime,
			Description:      md.Description,
			Label:            md.Label,
			LatestRevision:   md.LatestRevision,
			LatestExpireTime: md.LatestExpireTime,
			CreateTime:       md.CreateTime,
			UpdateTime:       md.UpdateTime,
		}
		toDrain, err := s.secretBackendService.GetRevisionsToDrain(ctx, model.UUID(s.model.UUID()), revisionMetadata[i])
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		for _, r := range toDrain {
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

// OwnerTagFromOwner returns the tag for a given secret owner.
func OwnerTagFromOwner(owner coresecrets.Owner) (names.Tag, error) {
	switch owner.Kind {
	case coresecrets.UnitOwner:
		return names.NewUnitTag(owner.ID), nil
	case coresecrets.ApplicationOwner:
		return names.NewApplicationTag(owner.ID), nil
	case coresecrets.ModelOwner:
		return names.NewModelTag(owner.ID), nil
	}
	return nil, errors.NotValidf("owner kind %q", owner.Kind)
}

func secretAccessorFromTag(authTag names.Tag) (secretservice.SecretAccessor, error) {
	switch authTag.(type) {
	case names.UnitTag:
		return secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor, ID: authTag.Id(),
		}, nil
	case names.ModelTag:
		return secretservice.SecretAccessor{
			Kind: secretservice.ModelAccessor, ID: authTag.Id(),
		}, nil
	}
	return secretservice.SecretAccessor{}, errors.NotValidf("auth tag kind %q", authTag.Kind())
}

// isLeaderUnit returns true if the authenticated caller is the unit leader of its application.
func isLeaderUnit(authTag names.Tag, leadershipChecker leadership.Checker) (bool, error) {
	appName, _ := names.UnitApplication(authTag.Id())
	token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
	err := token.Check()
	if err != nil && !leadership.IsNotLeaderError(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

func (s *SecretsDrainAPI) getCharmSecrets(ctx context.Context) ([]*coresecrets.SecretMetadata, [][]*coresecrets.SecretRevisionMetadata, error) {
	owners := []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   s.authTag.Id(),
	}}
	// Unit leaders can also get metadata for secrets owned by the app.
	isLeader, err := isLeaderUnit(s.authTag, s.leadershipChecker)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if isLeader {
		appName, _ := names.UnitApplication(s.authTag.Id())
		owners = append(owners, secretservice.CharmSecretOwner{
			Kind: secretservice.ApplicationOwner,
			ID:   appName,
		})
	}
	return s.secretService.ListCharmSecrets(ctx, owners...)
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
	accessor, err := secretAccessorFromTag(s.authTag)
	if err != nil {
		return
	}
	token, err := LeadershipToken(s.authTag, s.leadershipChecker)
	if err != nil {
		return errors.Trace(err)
	}
	return s.secretService.ChangeSecretBackend(ctx, uri, arg.Revision, toChangeSecretBackendParams(accessor, token, arg))
}

func toChangeSecretBackendParams(accessor secretservice.SecretAccessor, token leadership.Token, arg params.ChangeSecretBackendArg) secretservice.ChangeSecretBackendParams {
	params := secretservice.ChangeSecretBackendParams{
		LeaderToken: token,
		Accessor:    accessor,
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
