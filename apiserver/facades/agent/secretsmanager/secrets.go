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
	"github.com/juju/juju/core/params"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
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

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() && !context.Auth().AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// Work out the app name associated with the agent since this is
	// the secret owner for newly created secrets.
	agentTag := context.Auth().GetAuthTag()
	agentName := agentTag.Id()
	if agentTag.Kind() == names.UnitTagKind {
		agentName, _ = names.UnitApplication(agentName)
	}

	// For now we just support the Juju secrets provider.
	service, err := provider.NewSecretProvider(juju.Provider, secrets.ProviderConfig{
		juju.ParamBackend: context.State(),
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating juju secrets service")
	}
	return &SecretsManagerAPI{
		authOwner:       names.NewApplicationTag(agentName),
		controllerUUID:  context.State().ControllerUUID(),
		modelUUID:       context.State().ModelUUID(),
		secretsService:  service,
		resources:       context.Resources(),
		secretsRotation: context.State(),
		accessSecret:    secretAccessor(agentName),
	}, nil
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
	if !coresecrets.SecretStatus(arg.Status).IsValid() {
		return "", errors.NotValidf("secret status %q", arg.Status)
	}
	if len(arg.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
	}
	URL := coresecrets.NewSimpleURL(arg.Path)
	URL.ControllerUUID = s.controllerUUID
	URL.ModelUUID = s.modelUUID
	md, err := s.secretsService.CreateSecret(ctx, URL, secrets.CreateParams{
		Type:           coresecrets.SecretType(arg.Type),
		Version:        secrets.Version,
		Owner:          s.authOwner.String(),
		Path:           arg.Path,
		RotateInterval: arg.RotateInterval,
		Status:         coresecrets.SecretStatus(arg.Status),
		Description:    arg.Description,
		Tags:           arg.Tags,
		Params:         arg.Params,
		Data:           arg.Data,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URL.WithRevision(md.Revision).ShortString(), nil
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(args params.UpdateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		ID, err := s.updateSecret(ctx, arg)
		result.Results[i].Result = ID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func secretStatusPtr(in *string) *coresecrets.SecretStatus {
	if in == nil {
		return nil
	}
	s := coresecrets.SecretStatus(*in)
	return &s
}

func (s *SecretsManagerAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg) (string, error) {
	secretUrl, err := coresecrets.ParseURL(arg.URL)
	if err != nil {
		return "", errors.Trace(err)
	}
	if secretUrl.Attribute != "" {
		return "", errors.NotSupportedf("updating a single secret attribute %q", secretUrl.Attribute)
	}
	if secretUrl.Revision > 0 {
		return "", errors.NotSupportedf("updating secret revision %d", secretUrl.Revision)
	}
	if secretUrl.ControllerUUID != "" && secretUrl.ControllerUUID != s.controllerUUID {
		return "", errors.NotValidf("secret URL with controller UUID %q", secretUrl.ControllerUUID)
	}
	if secretUrl.ModelUUID != "" && secretUrl.ModelUUID != s.modelUUID {
		return "", errors.NotValidf("secret URL with model UUID %q", secretUrl.ModelUUID)
	}
	if arg.RotateInterval == nil && arg.Description == nil && arg.Status == nil &&
		arg.Tags == nil && len(arg.Params) == 0 && len(arg.Data) == 0 {
		return "", errors.New("at least one attribute to update must be specified")
	}
	if arg.RotateInterval != nil && *arg.RotateInterval < 0 {
		return "", errors.NotValidf("rotate interval %v", *arg.RotateInterval)
	}
	if arg.Status != nil && !coresecrets.SecretStatus(*arg.Status).IsValid() {
		return "", errors.NotValidf("secret status %q", arg.Status)
	}
	secretUrl.ControllerUUID = s.controllerUUID
	secretUrl.ModelUUID = s.modelUUID
	md, err := s.secretsService.UpdateSecret(ctx, secretUrl, secrets.UpdateParams{
		RotateInterval: arg.RotateInterval,
		Status:         secretStatusPtr(arg.Status),
		Description:    arg.Description,
		Tags:           arg.Tags,
		Params:         arg.Params,
		Data:           arg.Data,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URL.WithRevision(md.Revision).ShortString(), nil
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
	// TODO(wallyworld) - support ID
	if arg.URL == "" {
		return nil, errors.NotSupportedf("get secret by ID")
	}
	secretUrl, err := coresecrets.ParseURL(arg.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if secretUrl.ControllerUUID == "" {
		secretUrl.ControllerUUID = s.controllerUUID
	}
	if secretUrl.ModelUUID == "" {
		secretUrl.ModelUUID = s.modelUUID
	}
	val, err := s.secretsService.GetSecretValue(ctx, secretUrl)
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
					ID:             c.ID,
					URL:            c.URL.ID(),
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
	one := func(arg params.SecretRotatedArg) error {
		secretUrl, err := coresecrets.ParseURL(arg.URL)
		if err != nil {
			return errors.Trace(err)
		}
		if secretUrl.ModelUUID != "" && secretUrl.ModelUUID != s.modelUUID {
			return errors.NotValidf("secret URL with model UUID %q", secretUrl.ModelUUID)
		}
		secretUrl.ControllerUUID = s.controllerUUID
		secretUrl.ModelUUID = s.modelUUID
		app, ok := secretUrl.OwnerApplication()
		if !ok {
			return apiservererrors.ErrPerm
		}
		if !canAccess(names.NewApplicationTag(app)) {
			return apiservererrors.ErrPerm
		}
		return s.secretsRotation.SecretRotated(secretUrl, arg.When)
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}
