// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"

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

	accessSecret    common.GetAuthFunc
	secretsService  secrets.SecretsService
	resources       facade.Resources
	secretsRotation SecretsRotation
	authOwner       names.Tag
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		ID, err := s.createSecret(ctx, arg)
		result.Results[i].Result = ID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (string, error) {
	if arg.RotateInterval < 0 {
		return "", errors.NotValidf("rotate interval %q", arg.RotateInterval)
	}
	if len(arg.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
	}
	uri := coresecrets.NewURI()
	md, err := s.secretsService.CreateSecret(ctx, uri, secrets.CreateParams{
		Version:        secrets.Version,
		Owner:          s.authOwner.String(),
		RotateInterval: arg.RotateInterval,
		Description:    arg.Description,
		Tags:           arg.Tags,
		Params:         arg.Params,
		Data:           arg.Data,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URI.ShortString(), nil
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if uri.ControllerUUID != "" && uri.ControllerUUID != s.controllerUUID {
		return errors.NotValidf("secret URL with controller UUID %q", uri.ControllerUUID)
	}
	if arg.RotateInterval == nil && arg.Description == nil &&
		arg.Tags == nil && len(arg.Params) == 0 && len(arg.Data) == 0 {
		return errors.New("at least one attribute to update must be specified")
	}
	if arg.RotateInterval != nil && *arg.RotateInterval < 0 {
		return errors.NotValidf("rotate interval %v", *arg.RotateInterval)
	}
	uri.ControllerUUID = s.controllerUUID
	_, err = s.secretsService.UpdateSecret(ctx, uri, secrets.UpdateParams{
		RotateInterval: arg.RotateInterval,
		Description:    arg.Description,
		Tags:           arg.Tags,
		Params:         arg.Params,
		Data:           arg.Data,
	})
	return errors.Trace(err)
}

// GetSecretValues returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretValues(args params.GetSecretArgs) (params.SecretValueResults, error) {
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

func (s *SecretsManagerAPI) getSecretValue(ctx context.Context, arg params.GetSecretArg) (coresecrets.SecretData, error) {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if uri.ControllerUUID == "" {
		uri.ControllerUUID = s.controllerUUID
	}
	val, err := s.secretsService.GetSecretValue(ctx, uri, 1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return val.EncodedValues(), nil
}

// WatchSecretsRotationChanges sets up a watcher to notify of changes to secret rotation config.
func (s *SecretsManagerAPI) WatchSecretsRotationChanges(args params.Entities) (params.SecretRotationWatchResults, error) {
	canAccess, err := s.accessSecret()
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
	canAccess, err := s.accessSecret()
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
