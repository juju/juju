// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/state/watcher"
)

// SecretsManagerAPI is the implementation for the SecretsManager facade.
type SecretsManagerAPI struct {
	controllerUUID string
	modelUUID      string

	manageSecret    common.GetAuthFunc
	secretsService  secrets.SecretsService
	resources       facade.Resources
	secretsRotation SecretsRotation
	secretsConsumer SecretsConsumer
	authTag         names.Tag
	clock           clock.Clock
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	canManage, err := s.manageSecret()
	if err != nil {
		return params.StringResults{}, err
	}
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		ID, err := s.createSecret(ctx, arg, canManage)
		result.Results[i].Result = ID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(ctx context.Context, arg params.CreateSecretArg, canManage common.AuthFunc) (string, error) {
	if len(arg.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
	}
	// A unit can only create secrets owned by its app.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	if !canManage(secretOwner) {
		return "", apiservererrors.ErrPerm
	}
	uri := coresecrets.NewURI()
	md, err := s.secretsService.CreateSecret(ctx, uri, secrets.CreateParams{
		Version:      secrets.Version,
		Owner:        arg.OwnerTag,
		UpsertParams: fromUpsertParams(s.clock, arg.UpsertSecretArg),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URI.ShortString(), nil
}

func fromUpsertParams(clock clock.Clock, p params.UpsertSecretArg) secrets.UpsertParams {
	var nextRotateTime *time.Time
	if p.RotatePolicy != nil {
		// TODO(wallyworld) - we need to take into account last rotate time
		// This approximate will do for now.
		now := clock.Now()
		nextRotateTime = p.RotatePolicy.NextRotateTime(&now)
	}
	return secrets.UpsertParams{
		RotatePolicy:   p.RotatePolicy,
		NextRotateTime: nextRotateTime,
		ExpireTime:     p.ExpireTime,
		Description:    p.Description,
		Label:          p.Label,
		Params:         p.Params,
		Data:           p.Data,
	}
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(args params.UpdateSecretArgs) (params.ErrorResults, error) {
	canManage, err := s.manageSecret()
	if err != nil {
		return params.ErrorResults{}, err
	}
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg, canManage)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg, canManage common.AuthFunc) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if uri.ControllerUUID != "" && uri.ControllerUUID != s.controllerUUID {
		return errors.NotValidf("secret URI with controller UUID %q", uri.ControllerUUID)
	}
	if arg.RotatePolicy == nil && arg.Description == nil && arg.ExpireTime == nil &&
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Data) == 0 {
		return errors.New("at least one attribute to update must be specified")
	}
	uri.ControllerUUID = s.controllerUUID
	md, err := s.secretsService.GetSecret(ctx, uri)
	if err != nil {
		return errors.Trace(err)
	}
	secretOwner, err := names.ParseTag(md.OwnerTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canManage(secretOwner) {
		return apiservererrors.ErrPerm
	}
	_, err = s.secretsService.UpdateSecret(ctx, uri, fromUpsertParams(s.clock, arg.UpsertSecretArg))
	return errors.Trace(err)
}

// GetSecretValues returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretValues(args params.GetSecretValueArgs) (params.SecretValueResults, error) {
	result := params.SecretValueResults{
		Results: make([]params.SecretValueResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		data, err := s.getSecretValue(ctx, arg)
		result.Results[i].Data = data
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretValue(ctx context.Context, arg params.GetSecretValueArg) (coresecrets.SecretData, error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if uri.ControllerUUID == "" {
		uri.ControllerUUID = s.controllerUUID
	}
	if err := s.checkCanRead(uri); err != nil {
		return nil, apiservererrors.ErrPerm
	}
	consumer, err := s.secretsConsumer.GetSecretConsumer(uri, s.authTag.String())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	update := arg.Update || err != nil
	peek := arg.Peek
	if update || peek {
		md, err := s.secretsService.GetSecret(ctx, uri)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if consumer == nil {
			consumer = &coresecrets.SecretConsumerMetadata{}
		}
		consumer.CurrentRevision = md.Revision
		if arg.Label != "" {
			consumer.Label = arg.Label
		}

		if update {
			err := s.secretsConsumer.SaveSecretConsumer(uri, s.authTag.String(), consumer)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	val, err := s.secretsService.GetSecretValue(ctx, uri, consumer.CurrentRevision)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return val.EncodedValues(), nil
}

// WatchSecretsChanges sets up a watcher to notify of changes to secret revisions for the specified consumers.
func (s *SecretsManagerAPI) WatchSecretsChanges(args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []string, error) {
		_, err := names.ParseTag(arg.Tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		if s.authTag.String() != arg.Tag {
			return "", nil, apiservererrors.ErrPerm
		}
		w := s.secretsConsumer.WatchConsumedSecretsChanges(arg.Tag)
		if secretChanges, ok := <-w.Changes(); ok {
			changes := make([]string, len(secretChanges))
			for i, c := range secretChanges {
				changes[i] = c
			}
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.StringsWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.StringsWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// WatchSecretsRotationChanges sets up a watcher to notify of changes to secret rotation config.
func (s *SecretsManagerAPI) WatchSecretsRotationChanges(args params.Entities) (params.SecretRotationWatchResults, error) {
	canAccess, err := s.manageSecret()
	if err != nil {
		return params.SecretRotationWatchResults{}, err
	}

	results := params.SecretRotationWatchResults{
		Results: make([]params.SecretRotationWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []params.SecretRotationChange, error) {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil || !canAccess(ownerTag) {
			return "", nil, apiservererrors.ErrPerm
		}
		w := s.secretsRotation.WatchSecretsRotationChanges(ownerTag.String())
		if secretChanges, ok := <-w.Changes(); ok {
			changes := make([]params.SecretRotationChange, len(secretChanges))
			for i, c := range secretChanges {
				changes[i] = params.SecretRotationChange{
					URI:            c.URI.String(),
					RotateInterval: c.RotateInterval,
					LastRotateTime: c.LastRotateTime,
				}
			}
			return s.resources.Register(w), changes, nil
		}
		return "", nil, watcher.EnsureErr(w)
	}
	for i, arg := range args.Entities {
		var result params.SecretRotationWatchResult
		id, changes, err := one(arg)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.SecretRotationWatcherId = id
			result.Changes = changes
		}
		results.Results[i] = result
	}
	return results, nil
}

// SecretsRotated records when secrets were last rotated.
func (s *SecretsManagerAPI) SecretsRotated(args params.SecretRotatedArgs) (params.ErrorResults, error) {
	canAccess, err := s.manageSecret()
	if err != nil {
		return params.ErrorResults{}, err
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	one := func(arg params.SecretRotatedArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		uri.ControllerUUID = s.controllerUUID
		md, err := s.secretsService.GetSecret(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		owner, err := names.ParseTag(md.OwnerTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !canAccess(owner) {
			return apiservererrors.ErrPerm
		}
		return s.secretsRotation.SecretRotated(uri, arg.When)
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}
