// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	corewatcher "github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets"
	secretsprovider "github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

// CrossModelSecretsClient gets secret content from a cross model controller.
type CrossModelSecretsClient interface {
	GetRemoteSecretContentInfo(
		ctx context.Context, uri *coresecrets.URI, revision int, refresh, peek bool,
		sourceControllerUUID string, appUUID application.UUID, unitId int, macs macaroon.Slice,
	) (*secrets.ContentParams, *secretsprovider.ModelBackendConfig, int, bool, error)
	GetSecretAccessScope(ctx context.Context, uri *coresecrets.URI, appUUID application.UUID, unitId int) (relation.UUID, error)
	Close() error
}

// SecretsManagerAPI is the implementation for the SecretsManager facade.
type SecretsManagerAPI struct {
	authorizer                facade.Authorizer
	leadershipChecker         leadership.Checker
	secretBackendService      SecretBackendService
	secretService             SecretService
	watcherRegistry           facade.WatcherRegistry
	secretsTriggers           SecretTriggers
	secretsConsumer           SecretsConsumer
	applicationService        ApplicationService
	crossModelRelationService CrossModelRelationService
	authTag                   names.Tag
	clock                     clock.Clock
	controllerUUID            string
	modelUUID                 string

	remoteClientGetter func(ctx context.Context, uri *coresecrets.URI) (CrossModelSecretsClient, error)

	logger logger.Logger
}

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
	appName, _ := names.UnitApplication(s.authTag.Id())
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	cfgInfo, err := s.secretBackendService.DrainBackendConfigInfo(ctx, secretbackendservice.DrainBackendConfigParams{
		GrantedSecretsGetter: s.secretService.ListGrantedSecretsForBackend,
		LeaderToken:          token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   s.authTag.Id(),
		},
		ModelUUID: model.UUID(s.modelUUID),
		BackendID: backendID,
	})
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
	appName, _ := names.UnitApplication(s.authTag.Id())
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	cfgInfo, err := s.secretBackendService.BackendConfigInfo(ctx, secretbackendservice.BackendConfigParams{
		GrantedSecretsGetter: s.secretService.ListGrantedSecretsForBackend,
		LeaderToken:          token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   s.authTag.Id(),
		},
		ModelUUID:      model.UUID(s.modelUUID),
		BackendIDs:     backendIDs,
		SameController: true,
	})
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

func (s *SecretsManagerAPI) getBackend(ctx context.Context, backendID string, accessor secretservice.SecretAccessor, token leadership.Token) (*secretsprovider.ModelBackendConfig, bool, error) {
	cfgInfo, err := s.secretBackendService.BackendConfigInfo(ctx, secretbackendservice.BackendConfigParams{
		GrantedSecretsGetter: s.secretService.ListGrantedSecretsForBackend,
		LeaderToken:          token,
		Accessor:             accessor,
		ModelUUID:            model.UUID(s.modelUUID),
		BackendIDs:           []string{backendID},
		SameController:       true,
	})
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
		data, latestRevision, err := s.getSecretConsumerInfo(ctx, unitConsumer, uri)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = params.SecretConsumerInfoResult{
			Revision: latestRevision,
			Label:    data.Label,
		}
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretConsumerInfo(ctx context.Context, unitTag names.UnitTag, uriStr string) (*coresecrets.SecretConsumerMetadata, int, error) {
	uri, err := coresecrets.ParseURI(uriStr)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	unitName, err := unit.NewName(unitTag.Id())
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	return s.secretsConsumer.GetSecretConsumerAndLatest(ctx, uri, unitName)
}

func secretOwnersFromAuthTag(authTag names.Tag, leadershipChecker leadership.Checker) ([]secretservice.CharmSecretOwner, error) {
	owners := []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   authTag.Id(),
	}}
	// Unit leaders can also get metadata for secrets owned by the app.
	appName, _ := names.UnitApplication(authTag.Id())
	token := leadershipChecker.LeadershipCheck(appName, authTag.Id())
	err := token.Check()
	if err != nil && !leadership.IsNotLeaderError(err) {
		return nil, errors.Trace(err)
	}
	if err == nil {
		appName, _ := names.UnitApplication(authTag.Id())
		owners = append(owners, secretservice.CharmSecretOwner{
			Kind: secretservice.ApplicationOwner,
			ID:   appName,
		})
	}
	return owners, nil
}

// GetSecretMetadata returns metadata for the caller's secrets.
func (s *SecretsManagerAPI) GetSecretMetadata(ctx context.Context) (params.ListSecretMetadataResults, error) {
	var result params.ListSecretMetadataResults
	owners, err := secretOwnersFromAuthTag(s.authTag, s.leadershipChecker)
	if err != nil {
		return result, errors.Trace(err)
	}
	// TODO - use new service method to get metadata without revisions
	//  The facade API now returns params.ListSecretMetadataResults
	metadata, _, err := s.secretService.ListCharmSecrets(ctx, owners...)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, md := range metadata {
		ownerTag, err := commonsecrets.OwnerTagFromOwner(md.Owner)
		if err != nil {
			// This should never happen.
			return params.ListSecretMetadataResults{}, errors.Trace(err)
		}
		secretResult := params.ListSecretMetadataResult{
			URI:                    md.URI.String(),
			Version:                md.Version,
			OwnerTag:               ownerTag.String(),
			RotatePolicy:           md.RotatePolicy.String(),
			NextRotateTime:         md.NextRotateTime,
			Description:            md.Description,
			Label:                  md.Label,
			LatestRevision:         md.LatestRevision,
			LatestRevisionChecksum: md.LatestRevisionChecksum,
			LatestExpireTime:       md.LatestExpireTime,
			CreateTime:             md.CreateTime,
			UpdateTime:             md.UpdateTime,
		}
		grants, err := s.secretService.GetSecretGrants(ctx, md.URI, coresecrets.RoleView)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, g := range grants {
			accessorTag, err := tagFromSubject(g.Subject)
			if err != nil {
				return result, errors.Trace(err)
			}
			scopeTag, err := tagFromAccessScope(g.Scope)
			if err != nil {
				return result, errors.Trace(err)
			}
			secretResult.Access = append(secretResult.Access, params.AccessInfo{
				TargetTag: accessorTag.String(), ScopeTag: scopeTag.String(), Role: g.Role,
			})
		}
		result.Results = append(result.Results, secretResult)
	}
	return result, nil
}

func tagFromSubject(access secretservice.SecretAccessor) (names.Tag, error) {
	switch kind := access.Kind; kind {
	case secretservice.UnitAccessor:
		return names.NewUnitTag(access.ID), nil
	case secretservice.ApplicationAccessor:
		return names.NewApplicationTag(access.ID), nil
	case secretservice.ModelAccessor:
		return names.NewModelTag(access.ID), nil
	default:
		return nil, errors.NotValidf("subject kind %q", kind)
	}
}

func tagFromAccessScope(access secretservice.SecretAccessScope) (names.Tag, error) {
	switch kind := access.Kind; kind {
	case secretservice.UnitAccessScope:
		return names.NewUnitTag(access.ID), nil
	case secretservice.ApplicationAccessScope:
		return names.NewApplicationTag(access.ID), nil
	case secretservice.ModelAccessScope:
		return names.NewModelTag(access.ID), nil
	case secretservice.RelationAccessScope:
		return names.NewRelationTag(access.ID), nil
	default:
		return nil, errors.NotValidf("access scope kind %q", kind)
	}
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
			contentParams.Data = content.EncodedValues()
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
	s.logger.Debugf(ctx, "get remote secret %s", uri.String())
	extClient, err := s.remoteClientGetter(ctx, uri)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "creating remote secret client")
	}
	defer func() { _ = extClient.Close() }()

	unitName, err := unit.NewName(s.authTag.Id())
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	consumerApp, _ := names.UnitApplication(unitName.String())
	appUUID, err := s.applicationService.GetApplicationUUIDByName(ctx, consumerApp)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// Return an error that also matches a generic not found error.
		return nil, nil, false, internalerrors.Join(err, errors.Hide(errors.NotFound))
	} else if err != nil {
		return nil, nil, false, err
	}

	var unitId int
	if unitTag, ok := s.authTag.(names.UnitTag); ok {
		unitId = unitTag.Number()
	} else {
		return nil, nil, false, errors.NotSupportedf("getting cross model secret for consumer %q", s.authTag)
	}

	// Access scope for a CMR secret is always a relation.
	relationUUID, err := extClient.GetSecretAccessScope(ctx, uri, appUUID, unitId)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, nil, false, apiservererrors.ErrPerm
		}
		return nil, nil, false, errors.Trace(err)
	}
	s.logger.Debugf(ctx, "secret %q scoped to relation 5s for %v: %s", uri.String(), relationUUID, appUUID)

	mac, err := s.crossModelRelationService.GetMacaroonForRelation(ctx, relationUUID)
	if err != nil {
		return nil, nil, false, errors.Annotatef(err, "getting remote mac for relation %q", relationUUID)
	}
	macs := macaroon.Slice{mac}

	consumerInfo, err := s.secretsConsumer.GetSecretConsumer(ctx, uri, unitName)
	if err != nil && !errors.Is(err, secreterrors.SecretConsumerNotFound) {
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

	content, backend, latestRevision, draining, err := extClient.GetRemoteSecretContentInfo(
		ctx, uri, wantRevision, refresh, peek, s.controllerUUID, appUUID, unitId, macs)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}

	if refresh || labelToUpdate != nil {
		if refresh {
			consumerInfo.CurrentRevision = latestRevision
		}
		if labelToUpdate != nil {
			consumerInfo.Label = *labelToUpdate
		}
		if err := s.secretsConsumer.SaveSecretConsumer(ctx, uri, unitName, *consumerInfo); err != nil {
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
	accessor := secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   s.authTag.Id(),
	}
	appName, _ := names.UnitApplication(s.authTag.Id())
	token := s.leadershipChecker.LeadershipCheck(appName, s.authTag.Id())
	for i, rev := range arg.Revisions {
		// TODO(wallworld) - if pendingDelete is true, mark the revision for deletion
		val, valueRef, err := s.secretService.GetSecretValue(ctx, uri, rev, accessor)
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
			backend, draining, err := s.getBackend(ctx, valueRef.BackendID, accessor, token)
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

	unitName, err := unit.NewName(s.authTag.Id())
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	uri, labelToUpdate, err := s.secretService.ProcessCharmSecretConsumerLabel(ctx, unitName, uri, arg.Label)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}

	s.logger.Debugf(ctx, "getting secret content for: %s", uri)

	if !uri.IsLocal(s.modelUUID) {
		return s.getRemoteSecretContent(ctx, uri, arg.Refresh, arg.Peek, labelToUpdate)
	}

	// labelToUpdate is the consumer label for consumers.
	consumedRevision, err := s.secretsConsumer.GetConsumedRevision(ctx, uri, unitName, arg.Refresh, arg.Peek, labelToUpdate)
	if err != nil {
		return nil, nil, false, errors.Annotate(err, "getting latest secret revision")
	}

	accessor := secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   s.authTag.Id(),
	}
	val, valueRef, err := s.secretService.GetSecretValue(ctx, uri, consumedRevision, accessor)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, false, errors.Trace(err)
	}

	appName := unitName.Application()
	token := s.leadershipChecker.LeadershipCheck(appName, unitName.String())
	backend, draining, err := s.getBackend(ctx, content.ValueRef.BackendID, accessor, token)
	return content, backend, draining, errors.Trace(err)
}

// isSameApplication returns true if the authenticated entity and the specified entity are in the same application.
func isSameApplication(authTag names.Tag, tag names.Tag) bool {
	return appFromTag(authTag) == appFromTag(tag)
}

func appFromTag(tag names.Tag) string {
	switch tag.Kind() {
	case names.ApplicationTagKind:
		return tag.Id()
	case names.UnitTagKind:
		authAppName, _ := names.UnitApplication(tag.Id())
		return authAppName
	}
	return ""
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
			appName, _ := names.UnitApplication(authTag.Id())
			token := s.leadershipChecker.LeadershipCheck(appName, authTag.Id())
			if err := token.Check(); err != nil {
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
		unitName, err := unit.NewName(tag.Id())
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		w, err := s.secretsConsumer.WatchConsumedSecretsChanges(ctx, unitName)
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
//   - a secret revision owed by the entity no longer
//     has any consumers
//
// Obsolete revisions results are "uri/revno".
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

// WatchDeleted returns a watcher for notifying when:
//   - a secret owned by the entity is deleted
//   - a secret revision owed by the entity is deleted
//
// Deleted revisions results are "uri/revno" and deleted
// secret results are "uri".
func (s *SecretsManagerAPI) WatchDeleted(ctx context.Context, args params.Entities) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}

	owners, err := s.charmSecretOwnersFromArgs(s.authTag, args)
	if err != nil {
		return result, errors.Trace(err)
	}

	w, err := s.secretsTriggers.WatchDeleted(ctx, owners...)
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
		accessor := secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   s.authTag.Id(),
		}
		return s.secretsTriggers.SecretRotated(ctx, uri, secretservice.SecretRotatedParams{
			Accessor:         accessor,
			OriginalRevision: arg.OriginalRevision,
			Skip:             arg.Skip,
		})
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

// UnitOwnedSecretsAndRevisions returns all secret URIs and revision IDs for all
// secrets owned by the given unit.
func (s *SecretsManagerAPI) UnitOwnedSecretsAndRevisions(arg params.Entity) (params.SecretRevisionIDsResults, error) {
	var results params.SecretRevisionIDsResults
	unitTag, err := names.ParseUnitTag(arg.Tag)
	if err != nil {
		return results, apiservererrors.ErrPerm
	}

	// TOOO - implement me.
	_ = unitTag

	var info map[coresecrets.URI][]int
	results.Results = make([]params.SecretRevisionIDsResult, 0, len(info))
	for id, revs := range info {
		result := params.SecretRevisionIDsResult{
			URI:       id.String(),
			Revisions: revs,
		}
		results.Results = append(results.Results, result)
	}

	return results, nil
}

// OwnedSecretRevisions returns all the revision IDs for the given secret that
// is owned by either the unit or the unit's application.
func (s *SecretsManagerAPI) OwnedSecretRevisions(args params.SecretRevisionArgs) (params.SecretRevisionIDsResults, error) {
	unitTag, err := names.ParseUnitTag(args.Unit.Tag)
	if err != nil {
		return params.SecretRevisionIDsResults{}, apiservererrors.ErrPerm
	}
	results := params.SecretRevisionIDsResults{
		Results: make([]params.SecretRevisionIDsResult, len(args.SecretURIs)),
	}
	for i, secretID := range args.SecretURIs {
		uri, err := coresecrets.ParseURI(secretID)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// TODO - implement me.
		_ = unitTag
		_ = uri
		var revs []int
		results.Results[i] = params.SecretRevisionIDsResult{
			URI:       secretID,
			Revisions: revs,
		}
	}

	return results, nil
}
