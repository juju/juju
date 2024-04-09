// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	secretsprovider "github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// CrossModelSecretsClient gets secret content from a cross model controller.
type CrossModelSecretsClient interface {
	GetRemoteSecretContentInfo(ctx context.Context, uri *coresecrets.URI, revision int, refresh, peek bool, sourceControllerUUID, appToken string, unitId int, macs macaroon.Slice) (*secrets.ContentParams, *secretsprovider.ModelBackendConfig, int, bool, error)
	GetSecretAccessScope(uri *coresecrets.URI, appToken string, unitId int) (string, error)
	Close() error
}

// SecretsManagerAPI is the implementation for the SecretsManager facade.
type SecretsManagerAPI struct {
	authorizer        facade.Authorizer
	leadershipChecker leadership.Checker
	secretService     SecretService
	watcherRegistry   facade.WatcherRegistry
	secretsTriggers   SecretTriggers
	secretsConsumer   SecretsConsumer
	authTag           names.Tag
	clock             clock.Clock
	controllerUUID    string
	modelUUID         string

	backendConfigGetter commonsecrets.BackendConfigGetter
	adminConfigGetter   commonsecrets.BackendAdminConfigGetter
	drainConfigGetter   commonsecrets.BackendDrainConfigGetter
	remoteClientGetter  func(ctx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error)

	crossModelState CrossModelState

	logger loggo.Logger
}

// SecretsManagerAPIV1 the secrets manager facade v1.
// TODO - drop when we no longer support juju 3.1.0
type SecretsManagerAPIV1 struct {
	*SecretsManagerAPI
}

func (s *SecretsManagerAPI) canRead(ctx context.Context, uri *coresecrets.URI, unit names.UnitTag) (bool, error) {
	// First try looking up unit access.
	hasRole, err := s.secretsConsumer.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   unit.Id(),
	})
	if err != nil {
		// Typically not found error.
		return false, errors.Trace(err)
	}
	if hasRole.Allowed(coresecrets.RoleView) {
		return true, nil
	}

	// All units can read secrets owned by application.
	appName := commonsecrets.AuthTagApp(s.authTag)
	hasRole, err = s.secretsConsumer.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
		Kind: secretservice.ApplicationAccessor,
		ID:   appName,
	})
	if err != nil {
		// Typically not found error.
		return false, errors.Trace(err)
	}
	return hasRole.Allowed(coresecrets.RoleView), nil
}

// TODO(secrets) - move to the service
func (s *SecretsManagerAPI) canManage(ctx context.Context, uri *coresecrets.URI) (leadership.Token, error) {
	return commonsecrets.CanManage(ctx, s.secretsConsumer, s.leadershipChecker, s.authTag, uri)
}

// GetSecretStoreConfig is for 3.0.x agents.
// TODO - drop when we no longer support juju 3.0.x
func (s *SecretsManagerAPIV1) GetSecretStoreConfig(ctx context.Context) (params.SecretBackendConfig, error) {
	cfgInfo, err := s.GetSecretBackendConfig(ctx)
	if err != nil {
		return params.SecretBackendConfig{}, errors.Trace(err)
	}
	return cfgInfo.Configs[cfgInfo.ActiveID], nil
}

// GetSecretBackendConfig gets the config needed to create a client to secret backends.
// TODO - drop when we no longer support juju 3.1.x
func (s *SecretsManagerAPIV1) GetSecretBackendConfig(ctx context.Context) (params.SecretBackendConfigResultsV1, error) {
	cfgInfo, err := s.backendConfigGetter(ctx, nil, true)
	if err != nil {
		return params.SecretBackendConfigResultsV1{}, errors.Trace(err)
	}
	result := params.SecretBackendConfigResultsV1{
		ActiveID: cfgInfo.ActiveID,
		Configs:  make(map[string]params.SecretBackendConfig),
	}
	for id, cfg := range cfgInfo.Configs {
		result.ControllerUUID = cfg.ControllerUUID
		result.ModelUUID = cfg.ModelUUID
		result.ModelName = cfg.ModelName
		result.Configs[id] = params.SecretBackendConfig{
			BackendType: cfg.BackendType,
			Params:      cfg.Config,
		}
	}
	return result, nil
}

// GetSecretBackendConfigs isn't on the V1 API.
func (*SecretsManagerAPIV1) GetSecretBackendConfigs(ctx context.Context, _ struct{}) {}

// GetSecretBackendConfigs gets the config needed to create a client to secret backends.
func (s *SecretsManagerAPI) GetSecretBackendConfigs(ctx context.Context, arg params.SecretBackendArgs) (params.SecretBackendConfigResults, error) {
	if arg.ForDrain {
		return s.getBackendConfigForDrain(ctx, arg)
	}
	results := params.SecretBackendConfigResults{
		Results: make(map[string]params.SecretBackendConfigResult, len(arg.BackendIDs)),
	}
	result, activeID, err := s.getSecretBackendConfig(ctx, arg.BackendIDs)
	if err != nil {
		return results, errors.Trace(err)
	}
	results.ActiveID = activeID
	results.Results = result
	return results, nil
}

// GetBackendConfigForDrain fetches the config needed to make a secret backend client for the drain worker.
func (s *SecretsManagerAPI) getBackendConfigForDrain(ctx context.Context, arg params.SecretBackendArgs) (params.SecretBackendConfigResults, error) {
	if len(arg.BackendIDs) > 1 {
		return params.SecretBackendConfigResults{}, errors.Errorf("Maximumly only one backend ID can be specified for drain")
	}
	var backendID string
	if len(arg.BackendIDs) == 1 {
		backendID = arg.BackendIDs[0]
	}
	results := params.SecretBackendConfigResults{
		Results: make(map[string]params.SecretBackendConfigResult, 1),
	}
	cfgInfo, err := s.drainConfigGetter(ctx, backendID)
	if err != nil {
		return results, errors.Trace(err)
	}
	if len(cfgInfo.Configs) == 0 {
		return results, errors.NotFoundf("no secret backends available")
	}
	results.ActiveID = cfgInfo.ActiveID
	for id, cfg := range cfgInfo.Configs {
		results.Results[id] = params.SecretBackendConfigResult{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			Draining:       true,
			Config: params.SecretBackendConfig{
				BackendType: cfg.BackendType,
				Params:      cfg.Config,
			},
		}
	}
	return results, nil
}

// GetSecretBackendConfig gets the config needed to create a client to secret backends.
func (s *SecretsManagerAPI) getSecretBackendConfig(ctx context.Context, backendIDs []string) (map[string]params.SecretBackendConfigResult, string, error) {
	cfgInfo, err := s.backendConfigGetter(ctx, backendIDs, false)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	result := make(map[string]params.SecretBackendConfigResult)
	wanted := set.NewStrings(backendIDs...)
	for id, cfg := range cfgInfo.Configs {
		if len(wanted) > 0 {
			if !wanted.Contains(id) {
				continue
			}
		} else if id != cfgInfo.ActiveID {
			continue
		}
		result[id] = params.SecretBackendConfigResult{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			Draining:       id != cfgInfo.ActiveID,
			Config: params.SecretBackendConfig{
				BackendType: cfg.BackendType,
				Params:      cfg.Config,
			},
		}
	}
	return result, cfgInfo.ActiveID, nil
}

func (s *SecretsManagerAPI) getBackend(ctx context.Context, backendID string) (*secretsprovider.ModelBackendConfig, bool, error) {
	cfgInfo, err := s.backendConfigGetter(ctx, []string{backendID}, false)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[backendID]
	if ok {
		return &secretsprovider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: secretsprovider.BackendConfig{
				BackendType: cfg.BackendType,
				Config:      cfg.Config,
			},
		}, backendID != cfgInfo.ActiveID, nil
	}
	return nil, false, errors.NotFoundf("secret backend %q", backendID)
}

// CreateSecretURIs creates new secret URIs.
func (s *SecretsManagerAPI) CreateSecretURIs(ctx context.Context, arg params.CreateSecretURIsArg) (params.StringResults, error) {
	if arg.Count <= 0 {
		return params.StringResults{}, errors.NotValidf("secret URi count %d", arg.Count)
	}
	result := params.StringResults{
		Results: make([]params.StringResult, arg.Count),
	}
	uris, err := s.secretService.CreateSecretURIs(ctx, arg.Count)
	if err != nil {
		return params.StringResults{}, errors.Trace(err)
	}
	for i, uri := range uris {
		result.Results[i] = params.StringResult{Result: uri.String()}
	}
	return result, nil
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(ctx context.Context, args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		id, err := s.createSecret(ctx, arg)
		result.Results[i].Result = id
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (string, error) {
	if len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return "", errors.NotValidf("empty secret value")
	}
	// A unit can only create secrets owned by its app
	// if it is the leader.
	secretOwner, err := names.ParseTag(arg.OwnerTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	token, err := ownerToken(s.authTag, secretOwner, s.leadershipChecker)
	if err != nil {
		return "", errors.Trace(err)
	}
	var uri *coresecrets.URI
	if arg.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.URI)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		uri = coresecrets.NewURI()
	}

	params := secretservice.CreateSecretParams{
		Version:            secrets.Version,
		UpdateSecretParams: fromUpsertParams(arg.UpsertSecretArg, token),
	}
	switch kind := secretOwner.Kind(); kind {
	case names.UnitTagKind:
		params.CharmOwner = &secretservice.CharmSecretOwner{Kind: secretservice.UnitOwner, ID: secretOwner.Id()}
	case names.ApplicationTagKind:
		params.CharmOwner = &secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: secretOwner.Id()}
	default:
		return "", errors.NotValidf("secret owner kind %q", kind)
	}
	err = s.secretService.CreateSecret(ctx, uri, params)
	if err != nil {
		return "", errors.Trace(err)
	}
	return uri.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg, token leadership.Token) secretservice.UpdateSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return secretservice.UpdateSecretParams{
		LeaderToken:  token,
		RotatePolicy: p.RotatePolicy,
		ExpireTime:   p.ExpireTime,
		Description:  p.Description,
		Label:        p.Label,
		Params:       p.Params,
		Data:         p.Content.Data,
		ValueRef:     valueRef,
	}
}

// UpdateSecrets updates the specified secrets.
func (s *SecretsManagerAPI) UpdateSecrets(ctx context.Context, args params.UpdateSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg)
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) updateSecret(ctx context.Context, arg params.UpdateSecretArg) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	if arg.RotatePolicy == nil && arg.Description == nil && arg.ExpireTime == nil &&
		arg.Label == nil && len(arg.Params) == 0 && len(arg.Content.Data) == 0 && arg.Content.ValueRef == nil {
		return errors.New("at least one attribute to update must be specified")
	}

	token, err := s.canManage(ctx, uri)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = s.secretService.UpdateSecret(ctx, uri, fromUpsertParams(arg.UpsertSecretArg, token))
	return errors.Trace(err)
}

// RemoveSecrets removes the specified secrets.
func (s *SecretsManagerAPI) RemoveSecrets(ctx context.Context, args params.DeleteSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	if len(args.Args) == 0 {
		return result, nil
	}

	canDelete := func(uri *coresecrets.URI) error {
		_, err := s.canManage(ctx, uri)
		return errors.Trace(err)
	}

	for i, arg := range args.Args {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = s.secretService.DeleteCharmSecret(ctx, uri, arg.Revisions, canDelete)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// GetConsumerSecretsRevisionInfo returns the latest secret revisions for the specified secrets.
// This facade method is used for remote watcher to get the latest secret revisions and labels for a secret changed hook.
func (s *SecretsManagerAPI) GetConsumerSecretsRevisionInfo(ctx context.Context, args params.GetSecretConsumerInfoArgs) (params.SecretConsumerInfoResults, error) {
	result := params.SecretConsumerInfoResults{
		Results: make([]params.SecretConsumerInfoResult, len(args.URIs)),
	}
	consumerTag, err := names.ParseTag(args.ConsumerTag)
	if err != nil {
		return params.SecretConsumerInfoResults{}, errors.Trace(err)
	}
	unitConsumer, ok := consumerTag.(names.UnitTag)
	if !ok {
		return params.SecretConsumerInfoResults{}, errors.Errorf("expected unit tag for consumer %q, got %T", consumerTag, consumerTag)
	}
	for i, uri := range args.URIs {
		data, err := s.getSecretConsumerInfo(ctx, unitConsumer, uri)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = params.SecretConsumerInfoResult{
			Revision: data.LatestRevision,
			Label:    data.Label,
		}
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretConsumerInfo(ctx context.Context, unitTag names.UnitTag, uriStr string) (*coresecrets.SecretConsumerMetadata, error) {
	uri, err := coresecrets.ParseURI(uriStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We only check read permissions for local secrets.
	// For CMR secrets, the remote model manages the permissions.
	if uri.IsLocal(s.modelUUID) {
		canRead, err := s.canRead(ctx, uri, unitTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !canRead {
			return nil, apiservererrors.ErrPerm
		}
	}
	return s.secretsConsumer.GetSecretConsumer(ctx, uri, unitTag.Id())
}

func secretOwnersFromAuthTag(authTag names.Tag, leadershipChecker leadership.Checker) ([]secretservice.CharmSecretOwner, error) {
	owners := []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   authTag.Id(),
	}}
	// Unit leaders can also get metadata for secrets owned by the app.
	isLeader, err := isLeaderUnit(authTag, leadershipChecker)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if isLeader {
		appName := commonsecrets.AuthTagApp(authTag)
		owners = append(owners, secretservice.CharmSecretOwner{
			Kind: secretservice.ApplicationOwner,
			ID:   appName,
		})
	}
	return owners, nil
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata(ctx context.Context) (params.ListSecretResults, error) {
	var result params.ListSecretResults
	owners, err := secretOwnersFromAuthTag(s.authTag, s.leadershipChecker)
	if err != nil {
		return result, errors.Trace(err)
	}
	metadata, revisionMetadata, err := s.secretService.ListCharmSecrets(ctx, owners...)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, md := range metadata {
		ownerTag, err := commonsecrets.OwnerTagFromOwner(md.Owner)
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
		grants, err := s.secretService.GetSecretGrants(ctx, md.URI, coresecrets.RoleView)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, g := range grants {
			secretResult.Access = append(secretResult.Access, params.AccessInfo{
				TargetTag: g.Target, ScopeTag: g.Scope, Role: g.Role,
			})
		}

		for _, r := range revisionMetadata[i] {
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

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretContentInfo(ctx context.Context, args params.GetSecretContentArgs) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		content, backend, draining, err := s.getSecretContent(ctx, arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		contentParams := params.SecretContentParams{}
		if content.ValueRef != nil {
			contentParams.ValueRef = &params.SecretValueRef{
				BackendID:  content.ValueRef.BackendID,
				RevisionID: content.ValueRef.RevisionID,
			}
		}
		if content.SecretValue != nil {
			contentParams.Data = content.SecretValue.EncodedValues()
		}
		result.Results[i].Content = contentParams
		if backend != nil {
			result.Results[i].BackendConfig = &params.SecretBackendConfigResult{
				ControllerUUID: backend.ControllerUUID,
				ModelUUID:      backend.ModelUUID,
				ModelName:      backend.ModelName,
				Draining:       draining,
				Config: params.SecretBackendConfig{
					BackendType: backend.BackendType,
					Params:      backend.Config,
				},
			}
		}
	}
	return result, nil
}

func (s *SecretsManagerAPI) getRemoteSecretContent(ctx context.Context, uri *coresecrets.URI, refresh, peek bool, labelToUpdate *string) (
	*secrets.ContentParams, *secretsprovider.ModelBackendConfig, bool, error,
) {
	extClient, err := s.remoteClientGetter(ctx, uri)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "creating remote secret client")
	}
	defer func() { _ = extClient.Close() }()

	consumerApp := commonsecrets.AuthTagApp(s.authTag)
	token, err := s.crossModelState.GetToken(names.NewApplicationTag(consumerApp))
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote token for %q", consumerApp)
	}
	var unitId int
	if unitTag, ok := s.authTag.(names.UnitTag); ok {
		unitId = unitTag.Number()
	} else {
		return nil, nil, false, errors.NotSupportedf("getting cross model secret for consumer %q", s.authTag)
	}

	unitName := s.authTag.Id()
	consumerInfo, err := s.secretsConsumer.GetSecretConsumer(ctx, uri, unitName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, nil, false, errors.Trace(err)
	}
	var wantRevision int
	if err == nil {
		wantRevision = consumerInfo.CurrentRevision
	} else {
		// Not found so need to create a new record and populate
		// with latest revision.
		refresh = true
		consumerInfo = &coresecrets.SecretConsumerMetadata{}
	}

	scopeToken, err := extClient.GetSecretAccessScope(uri, token, unitId)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil, false, apiservererrors.ErrPerm
		}
		return nil, nil, false, errors.Trace(err)
	}
	s.logger.Debugf("secret %q scope token for %v: %s", uri.String(), token, scopeToken)

	scopeEntity, err := s.crossModelState.GetRemoteEntity(scopeToken)
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote entity for %q", scopeToken)
	}
	s.logger.Debugf("secret %q scope for %v: %s", uri.String(), scopeToken, scopeEntity)

	mac, err := s.crossModelState.GetMacaroon(scopeEntity)
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote mac for %q", scopeEntity)
	}

	macs := macaroon.Slice{mac}
	content, backend, latestRevision, draining, err := extClient.GetRemoteSecretContentInfo(ctx, uri, wantRevision, refresh, peek, s.controllerUUID, token, unitId, macs)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	if refresh || labelToUpdate != nil {
		if refresh {
			consumerInfo.LatestRevision = latestRevision
			consumerInfo.CurrentRevision = latestRevision
		}
		if labelToUpdate != nil {
			consumerInfo.Label = *labelToUpdate
		}
		if err := s.secretsConsumer.SaveSecretConsumer(ctx, uri, unitName, consumerInfo); err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}
	return content, backend, draining, nil
}

// GetSecretRevisionContentInfo returns the secret values for the specified secret revisions.
func (s *SecretsManagerAPI) GetSecretRevisionContentInfo(ctx context.Context, arg params.SecretRevisionArg) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(arg.Revisions)),
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return params.SecretContentResults{}, errors.Trace(err)
	}
	if _, err = s.canManage(ctx, uri); err != nil {
		return params.SecretContentResults{}, errors.Trace(err)
	}
	for i, rev := range arg.Revisions {
		// TODO(wallworld) - if pendingDelete is true, mark the revision for deletion
		val, valueRef, err := s.secretService.GetSecretValue(ctx, uri, rev)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		contentParams := params.SecretContentParams{}
		if valueRef != nil {
			contentParams.ValueRef = &params.SecretValueRef{
				BackendID:  valueRef.BackendID,
				RevisionID: valueRef.RevisionID,
			}
			backend, draining, err := s.getBackend(ctx, valueRef.BackendID)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			result.Results[i].BackendConfig = &params.SecretBackendConfigResult{
				ControllerUUID: backend.ControllerUUID,
				ModelUUID:      backend.ModelUUID,
				ModelName:      backend.ModelName,
				Draining:       draining,
				Config: params.SecretBackendConfig{
					BackendType: backend.BackendType,
					Params:      backend.Config,
				},
			}
		}
		if val != nil {
			contentParams.Data = val.EncodedValues()
		}
		result.Results[i].Content = contentParams
	}
	return result, nil
}

func (s *SecretsManagerAPI) canUpdateAppOwnedOrUnitOwnedSecretLabel(owner coresecrets.Owner) (bool, error) {
	if owner.ID != s.authTag.Id() || owner.Kind != coresecrets.UnitOwner {
		isLeaderUnit, err := commonsecrets.IsLeaderUnit(s.authTag, s.leadershipChecker)
		if err != nil {
			return false, errors.Trace(err)
		}
		// Only unit leaders can update app owned secret labels.
		if !isLeaderUnit {
			return false, nil
		}
	}
	return true, nil
}

func (s *SecretsManagerAPI) checkCallerOwner(owner coresecrets.Owner) (bool, leadership.Token, error) {
	isOwner, err := s.canUpdateAppOwnedOrUnitOwnedSecretLabel(owner)
	if err != nil {
		return false, nil, errors.Trace(err)
	}
	if !isOwner {
		return false, nil, nil
	}
	var ownerTag names.Tag
	if owner.Kind == coresecrets.UnitOwner {
		ownerTag = names.NewUnitTag(owner.ID)
	} else {
		ownerTag = names.NewApplicationTag(owner.ID)
	}
	token, err := ownerToken(s.authTag, ownerTag, s.leadershipChecker)
	return isOwner, token, errors.Trace(err)
}

func (s *SecretsManagerAPI) getSecretContent(ctx context.Context, arg params.GetSecretContentArg) (
	*secrets.ContentParams, *secretsprovider.ModelBackendConfig, bool, error,
) {
	// Only the owner can access secrets via the secret metadata label added by the owner.
	// (Note: the leader unit is not the owner of the application secrets).
	// Consumers get to use their own label.
	// Both owners and consumers can also just use the secret URI.

	if arg.URI == "" && arg.Label == "" {
		return nil, nil, false, errors.NewNotValid(nil, "both uri and label are empty")
	}

	var uri *coresecrets.URI
	var err error

	if arg.URI != "" {
		uri, err = coresecrets.ParseURI(arg.URI)
		if err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}

	unitName := s.authTag.Id()
	uri, labelToUpdate, err := s.secretService.ProcessSecretConsumerLabel(ctx, unitName, uri, arg.Label, s.checkCallerOwner)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}

	s.logger.Debugf("getting secret content for: %s", uri)

	if !uri.IsLocal(s.modelUUID) {
		return s.getRemoteSecretContent(ctx, uri, arg.Refresh, arg.Peek, labelToUpdate)
	}

	canRead, err := s.canRead(ctx, uri, s.authTag.(names.UnitTag))
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	if !canRead {
		return nil, nil, false, apiservererrors.ErrPerm
	}

	// labelToUpdate is the consumer label for consumers.
	consumedRevision, err := s.secretsConsumer.GetConsumedRevision(ctx, uri, s.authTag.Id(), arg.Refresh, arg.Peek, labelToUpdate)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "getting latest secret revision")
	}

	val, valueRef, err := s.secretService.GetSecretValue(ctx, uri, consumedRevision)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, false, errors.Trace(err)
	}
	backend, draining, err := s.getBackend(ctx, content.ValueRef.BackendID)
	return content, backend, draining, errors.Trace(err)
}

// UpdateTrackedRevisions updates the consumer info to track the latest
// revisions for the specified secrets.
func (s *SecretsManagerAPI) UpdateTrackedRevisions(ctx context.Context, uris []string) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(uris)),
	}
	for i, uriStr := range uris {
		uri, err := coresecrets.ParseURI(uriStr)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		_, err = s.secretsConsumer.GetConsumedRevision(ctx, uri, s.authTag.Id(), true, false, nil)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) charmSecretOwnersFromArgs(authTag names.Tag, args params.Entities) ([]secretservice.CharmSecretOwner, error) {
	var result []secretservice.CharmSecretOwner
	for _, arg := range args.Entities {
		ownerTag, err := names.ParseTag(arg.Tag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !isSameApplication(authTag, ownerTag) {
			return result, apiservererrors.ErrPerm
		}
		owner := secretservice.CharmSecretOwner{
			Kind: secretservice.UnitOwner,
			ID:   ownerTag.Id(),
		}
		// Only unit leaders can watch application secrets.
		if ownerTag.Kind() == names.ApplicationTagKind {
			_, err := commonsecrets.LeadershipToken(authTag, s.leadershipChecker)
			if err != nil {
				return result, errors.Trace(err)
			}
			owner.Kind = secretservice.ApplicationOwner
		}
		result = append(result, owner)
	}
	return result, nil
}

// WatchConsumedSecretsChanges sets up a watcher to notify of changes to secret revisions for the specified consumers.
func (s *SecretsManagerAPI) WatchConsumedSecretsChanges(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (string, []string, error) {
		tag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		if !isSameApplication(s.authTag, tag) {
			return "", nil, apiservererrors.ErrPerm
		}
		w, err := s.secretsConsumer.WatchConsumedSecretsChanges(ctx, tag.Id())
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		id, changes, err := internal.EnsureRegisterWatcher[[]string](ctx, s.watcherRegistry, w)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		return id, changes, nil
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

// WatchObsolete returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *SecretsManagerAPI) WatchObsolete(ctx context.Context, args params.Entities) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}

	owners, err := s.charmSecretOwnersFromArgs(s.authTag, args)
	if err != nil {
		return result, errors.Trace(err)
	}

	w, err := s.secretsTriggers.WatchObsolete(ctx, owners...)
	if err != nil {
		return result, errors.Trace(err)
	}
	id, changes, err := internal.EnsureRegisterWatcher[[]string](ctx, s.watcherRegistry, w)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	result.StringsWatcherId = id
	result.Changes = changes
	return result, nil
}

// WatchSecretsRotationChanges sets up a watcher to notify of changes to secret rotation config.
func (s *SecretsManagerAPI) WatchSecretsRotationChanges(ctx context.Context, args params.Entities) (params.SecretTriggerWatchResult, error) {
	result := params.SecretTriggerWatchResult{}

	owners, err := s.charmSecretOwnersFromArgs(s.authTag, args)
	if err != nil {
		return result, errors.Trace(err)
	}

	w, err := s.secretsTriggers.WatchSecretsRotationChanges(ctx, owners...)
	if err != nil {
		return result, errors.Trace(err)
	}

	id, secretChanges, err := internal.EnsureRegisterWatcher[[]corewatcher.SecretTriggerChange](ctx, s.watcherRegistry, w)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	changes := make([]params.SecretTriggerChange, len(secretChanges))
	for i, c := range secretChanges {
		changes[i] = params.SecretTriggerChange{
			URI:             c.URI.ID,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	result.WatcherId = id
	result.Changes = changes
	return result, nil
}

// SecretsRotated records when secrets were last rotated.
func (s *SecretsManagerAPI) SecretsRotated(ctx context.Context, args params.SecretRotatedArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	one := func(arg params.SecretRotatedArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		md, err := s.secretService.GetSecret(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		owner, err := commonsecrets.OwnerTagFromOwner(md.Owner)
		if err != nil {
			return errors.Trace(err)
		}
		if commonsecrets.AuthTagApp(s.authTag) != owner.Id() {
			return apiservererrors.ErrPerm
		}
		return s.secretsTriggers.SecretRotated(ctx, uri, arg.OriginalRevision, arg.Skip)
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}

// WatchSecretRevisionsExpiryChanges sets up a watcher to notify of changes to secret revision expiry config.
func (s *SecretsManagerAPI) WatchSecretRevisionsExpiryChanges(ctx context.Context, args params.Entities) (params.SecretTriggerWatchResult, error) {
	result := params.SecretTriggerWatchResult{}

	owners, err := s.charmSecretOwnersFromArgs(s.authTag, args)
	if err != nil {
		return result, errors.Trace(err)
	}

	w, err := s.secretsTriggers.WatchSecretRevisionsExpiryChanges(ctx, owners...)
	if err != nil {
		return result, errors.Trace(err)
	}
	id, secretChanges, err := internal.EnsureRegisterWatcher[[]corewatcher.SecretTriggerChange](ctx, s.watcherRegistry, w)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	changes := make([]params.SecretTriggerChange, len(secretChanges))
	for i, c := range secretChanges {
		changes[i] = params.SecretTriggerChange{
			URI:             c.URI.ID,
			Revision:        c.Revision,
			NextTriggerTime: c.NextTriggerTime,
		}
	}
	result.WatcherId = id
	result.Changes = changes
	return result, nil
}

type grantRevokeFunc func(context.Context, *coresecrets.URI, secretservice.SecretAccessParams) error

// SecretsGrant grants access to a secret for the specified subjects.
func (s *SecretsManagerAPI) SecretsGrant(ctx context.Context, args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(ctx, args, s.secretsConsumer.GrantSecretAccess)
}

// SecretsRevoke revokes access to a secret for the specified subjects.
func (s *SecretsManagerAPI) SecretsRevoke(ctx context.Context, args params.GrantRevokeSecretArgs) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(ctx, args, s.secretsConsumer.RevokeSecretAccess)
}

func permissionIDFromTag(tag names.Tag) (permission.ID, error) {
	switch kind := tag.Kind(); kind {
	case names.ApplicationTagKind:
		return permission.ID{ObjectType: permission.Application, Key: tag.Id()}, nil
	case names.UnitTagKind:
		return permission.ID{ObjectType: permission.Unit, Key: tag.Id()}, nil
	case names.RelationTagKind:
		return permission.ID{ObjectType: permission.Relation, Key: tag.Id()}, nil
	default:
		return permission.ID{}, errors.Errorf("tag kind %q not valid for secret permission", kind)
	}
}

func (s *SecretsManagerAPI) secretsGrantRevoke(ctx context.Context, args params.GrantRevokeSecretArgs, op grantRevokeFunc) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	one := func(arg params.GrantRevokeSecretArg) error {
		uri, err := coresecrets.ParseURI(arg.URI)
		if err != nil {
			return errors.Trace(err)
		}
		var scopeID permission.ID
		if arg.ScopeTag != "" {
			scopeTag, err := names.ParseTag(arg.ScopeTag)
			if err != nil {
				return errors.Trace(err)
			}
			scopeID, err = permissionIDFromTag(scopeTag)
			if err != nil {
				return errors.Trace(err)
			}
		}
		role := coresecrets.SecretRole(arg.Role)
		if role != "" && !role.IsValid() {
			return errors.NotValidf("secret role %q", arg.Role)
		}
		token, err := s.canManage(ctx, uri)
		if err != nil {
			return errors.Trace(err)
		}
		for _, tagStr := range arg.SubjectTags {
			subjectTag, err := names.ParseTag(tagStr)
			if err != nil {
				return errors.Trace(err)
			}
			subjectID, err := permissionIDFromTag(subjectTag)
			if err != nil {
				return errors.Trace(err)
			}
			if err := op(ctx, uri, secretservice.SecretAccessParams{
				LeaderToken: token,
				Scope:       scopeID,
				Subject:     subjectID,
				Role:        role,
			}); err != nil {
				return errors.Annotatef(err, "cannot change access to %q for %q", uri, tagStr)
			}
		}
		return nil
	}
	for i, arg := range args.Args {
		var result params.ErrorResult
		result.Error = apiservererrors.ServerError(one(arg))
		results.Results[i] = result
	}
	return results, nil
}
