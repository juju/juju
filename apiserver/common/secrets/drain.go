// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/rpc/params"
)

// SecretsDrainAPI is the implementation for the SecretsDrain facade.
type SecretsDrainAPI struct {
	authTag           names.Tag
	logger            logger.Logger
	leadershipChecker leadership.Checker
	watcherRegistry   facade.WatcherRegistry

	modelUUID            model.UUID
	secretService        SecretService
	secretBackendService SecretBackendService
}

// NewSecretsDrainAPI returns a new SecretsDrainAPI.
func NewSecretsDrainAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
	logger logger.Logger,
	leadershipChecker leadership.Checker,
	modelUUID model.UUID,
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
		modelUUID:            modelUUID,
		secretService:        secretService,
		secretBackendService: secretBackendService,
		watcherRegistry:      watcherRegistry,
	}, nil
}

// GetSecretsToDrain returns metadata for the secrets that need to be drained.
func (s *SecretsDrainAPI) GetSecretsToDrain(ctx context.Context) (params.SecretRevisionsToDrainResults, error) {
	var (
		toDrain []*coresecrets.SecretMetadataForDrain
		err     error
	)
	if s.authTag.Kind() == names.ModelTagKind {
		toDrain, err = s.secretService.ListUserSecretsToDrain(ctx)
	} else {
		toDrain, err = s.getCharmSecretsToDrain(ctx)
	}
	if err != nil {
		return params.SecretRevisionsToDrainResults{}, errors.Trace(err)
	}

	var result params.SecretRevisionsToDrainResults
	for _, info := range toDrain {
		secretResult := params.SecretRevisionsToDrainResult{
			URI: info.URI.String(),
		}
		toDrain, err := s.secretBackendService.GetRevisionsToDrain(ctx, s.modelUUID, info.Revisions)
		if err != nil {
			return params.SecretRevisionsToDrainResults{}, errors.Trace(err)
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

func secretAccessorFromTag(authTag names.Tag) (secret.SecretAccessor, error) {
	switch authTag.(type) {
	case names.UnitTag:
		return secret.SecretAccessor{
			Kind: secret.UnitAccessor, ID: authTag.Id(),
		}, nil
	case names.ModelTag:
		return secret.SecretAccessor{
			Kind: secret.ModelAccessor, ID: authTag.Id(),
		}, nil
	}
	return secret.SecretAccessor{}, errors.NotValidf("auth tag kind %q", authTag.Kind())
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

func (s *SecretsDrainAPI) getCharmSecretsToDrain(ctx context.Context) ([]*coresecrets.SecretMetadataForDrain, error) {
	owners := []secret.CharmSecretOwner{{
		Kind: secret.UnitCharmSecretOwner,
		ID:   s.authTag.Id(),
	}}
	// Unit leaders can also get metadata for secrets owned by the app.
	isLeader, err := isLeaderUnit(s.authTag, s.leadershipChecker)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if isLeader {
		appName, _ := names.UnitApplication(s.authTag.Id())
		owners = append(owners, secret.CharmSecretOwner{
			Kind: secret.ApplicationCharmSecretOwner,
			ID:   appName,
		})
	}
	return s.secretService.ListCharmSecretsToDrain(ctx, owners...)
}

// ChangeSecretBackend updates the backend for the specified secret after migration done.
func (s *SecretsDrainAPI) ChangeSecretBackend(ctx context.Context, args params.ChangeSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.changeSecretBackendForOne(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(codedSecretError(err))
	}
	return result, nil
}

// codedSecretError translates well-known secret domain errors into coded
// params errors at the facade boundary, preserving the message. Other
// errors are returned unchanged.
func codedSecretError(err error) error {
	switch {
	case errors.Is(err, secreterrors.SecretNotFound):
		return apiservererrors.ParamsErrorf(params.CodeSecretNotFound, "%s", err.Error())
	case errors.Is(err, secreterrors.PermissionDenied):
		return apiservererrors.ParamsErrorf(params.CodeUnauthorized, "%s", err.Error())
	}
	return err
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
	return s.secretService.ChangeSecretBackend(ctx, uri, arg.Revision, toChangeSecretBackendParams(accessor, arg))
}

func toChangeSecretBackendParams(accessor secret.SecretAccessor, arg params.ChangeSecretBackendArg) secretservice.ChangeSecretBackendParams {
	params := secretservice.ChangeSecretBackendParams{
		Accessor: accessor,
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

// WatchSecretBackendChanged sets up a watcher to notify of changes to the secret backend.
func (s *SecretsDrainAPI) WatchSecretBackendChanged(ctx context.Context) (params.NotifyWatchResult, error) {
	w, err := s.secretBackendService.WatchModelSecretBackendChanged(ctx, s.modelUUID)
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
