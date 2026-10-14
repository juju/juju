// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	crossmodelbakery "github.com/juju/juju/apiserver/internal/crossmodel/bakery"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
)

// CrossModelSecretsAPIV1 provides access to the CrossModelSecrets API V1 facade.
type CrossModelSecretsAPIV1 struct {
	*CrossModelSecretsAPI
}

// CrossModelSecretsAPI provides access to the CrossModelSecrets API facade.
type CrossModelSecretsAPI struct {
	mu sync.Mutex

	controllerUUID string
	modelUUID      model.UUID
	logger         logger.Logger

	auth facade.CrossModelAuthContext

	secretBackendService            SecretBackendService
	secretServiceGetter             func(c context.Context, modelUUID model.UUID) (SecretService, error)
	crossModelRelationServiceGetter func(c context.Context, modelUUID model.UUID) (CrossModelRelationService, error)
}

// NewCrossModelSecretsAPI returns a new server-side CrossModelSecretsAPI facade.
func NewCrossModelSecretsAPI(
	controllerUUID string,
	modelUUID model.UUID,
	auth facade.CrossModelAuthContext,
	secretBackendService SecretBackendService,
	secretServiceGetter func(c context.Context, modelUUID model.UUID) (SecretService, error),
	crossModelRelationServiceGetter func(c context.Context, modelUUID model.UUID) (CrossModelRelationService, error),
	logger logger.Logger,
) (*CrossModelSecretsAPI, error) {
	return &CrossModelSecretsAPI{
		controllerUUID:                  controllerUUID,
		modelUUID:                       modelUUID,
		auth:                            auth,
		secretBackendService:            secretBackendService,
		secretServiceGetter:             secretServiceGetter,
		crossModelRelationServiceGetter: crossModelRelationServiceGetter,
		logger:                          logger,
	}, nil
}

// GetSecretAccessScope returns the tokens for the access scope of the specified secrets and consumers.
func (s *CrossModelSecretsAPI) GetSecretAccessScope(ctx context.Context, args params.GetRemoteSecretAccessArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		relUUID, err := s.getSecretAccessScope(ctx, arg)
		if err != nil {
			switch {
			case errors.Is(err, secreterrors.SecretNotFound):
				err = apiservererrors.ParamsErrorf(params.CodeNotFound, "secret %q not found", arg.URI)
			case errors.Is(err, secreterrors.SecretAccessScopeNotFound):
				err = apiservererrors.ParamsErrorf(params.CodeNotFound, "secret access scope not found")
			}
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue

		}
		result.Results[i].Result = relUUID
	}
	return result, nil
}

func (s *CrossModelSecretsAPI) getSecretAccessScope(ctx context.Context, arg params.GetRemoteSecretAccessArg) (string, error) {
	if arg.URI == "" {
		return "", errors.Errorf("empty uri not valid").Add(coreerrors.NotValid)
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return "", errors.Capture(err)
	}
	if uri.SourceUUID == "" {
		return "", errors.Errorf("secret URI with empty source UUID not valid").Add(coreerrors.NotValid)
	}

	crossModelService, err := s.crossModelRelationServiceGetter(ctx, model.UUID(uri.SourceUUID))
	if err != nil {
		return "", errors.Capture(err)
	}

	// The consuming application may hold several offer connections in this
	// model, each represented by its own synthetic application, so the
	// application token alone cannot identify the connection the secret
	// was granted over. The grant of the secret resolves both the
	// connection and the relation scoping it, which is the access scope
	// to report.
	grants, err := crossModelService.GetRemoteConsumerSecretGrants(ctx, uri, coreapplication.UUID(arg.ApplicationToken))
	if err != nil {
		return "", errors.Capture(err)
	}

	// The grants are ordered by the name of the synthetic application, so
	// the choice is deterministic.
	relationUUID := grants[0].RelationUUID

	s.logger.Debugf(ctx, "access scope for secret %v and consuming application UUID %q: %v",
		uri.String(), arg.ApplicationToken, relationUUID)
	return relationUUID.String(), nil
}

// marshallLegacyBackendConfig converts the supplied backend config
// so it is suitable for older juju agents.
func marshallLegacyBackendConfig(cfg params.SecretBackendConfig) error {
	if cfg.BackendType != kubernetes.BackendType {
		return nil
	}
	if _, ok := cfg.Params["credential"]; ok {
		return nil
	}
	token, ok := cfg.Params["token"].(string)
	if !ok {
		return nil
	}
	delete(cfg.Params, "token")
	delete(cfg.Params, "namespace")
	delete(cfg.Params, "prefer-incluster-address")

	cred := cloud.NewCredential(cloud.OAuth2AuthType, map[string]string{k8scloud.CredAttrToken: token})
	credData, err := json.Marshal(cred)
	if err != nil {
		return errors.Errorf("error marshalling backend config: %w", err)
	}
	cfg.Params["credential"] = string(credData)
	cfg.Params["is-controller-cloud"] = false
	return nil
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPIV1) GetSecretContentInfo(ctx context.Context, args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
	results, err := s.CrossModelSecretsAPI.GetSecretContentInfo(ctx, args)
	if err != nil {
		return params.SecretContentResults{}, errors.Capture(err)
	}
	for i, cfg := range results.Results {
		if cfg.BackendConfig == nil {
			continue
		}
		if err := marshallLegacyBackendConfig(cfg.BackendConfig.Config); err != nil {
			return params.SecretContentResults{}, errors.Errorf("marshalling legacy backend config: %w", err)
		}
		results.Results[i] = cfg
	}
	return results, nil
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPI) GetSecretContentInfo(ctx context.Context, args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		content, backend, latestRevision, err := s.getSecretContent(ctx, arg)
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
		result.Results[i].BackendConfig = backend
		result.Results[i].LatestRevision = &latestRevision
	}
	return result, nil
}

func (s *CrossModelSecretsAPI) checkRelationMacaroons(
	ctx context.Context, crossModelRelationService CrossModelRelationService, appName string,
	mac macaroon.Slice, version bakery.Version,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check that the macaroon contains caveats for the relevant offer and
	// relation and that the consumer is in the relation.
	relKey, offerUUID, ok := crossmodelbakery.RelationInfoFromMacaroons(mac)
	if !ok {
		s.logger.Debugf(ctx, "missing relation or offer uuid from macaroons for consumer %v", appName)
		return apiservererrors.ErrPerm
	}
	key, err := relation.NewKeyFromString(relKey)
	if err != nil {
		return errors.Errorf("invalid relation key %q: %w", relKey, err)
	}
	valid, err := crossModelRelationService.IsCrossModelRelationValidForApplication(ctx, key, appName)
	if errors.Is(err, relationerrors.RelationNotFound) {
		return apiservererrors.ParamsErrorf(params.CodeNotFound, "relation %q not found", key)
	} else if err != nil {
		return errors.Capture(err)
	}
	if !valid {
		s.logger.Debugf(ctx, "secret consumer %q for relation %q not valid", appName, relKey)
		return apiservererrors.ErrPerm
	}

	// A cross model secret can only be accessed if the corresponding cross model relation
	// it is scoped to is accessible by the supplied macaroon.
	relationTag := names.NewRelationTag(key.String())
	err = s.auth.Authenticator().CheckRelationMacaroons(ctx, s.modelUUID.String(), offerUUID, relationTag, mac, version)
	return err
}

func (s *CrossModelSecretsAPI) getSecretContent(ctx context.Context, arg params.GetRemoteSecretContentArg) (*secrets.ContentParams, *params.SecretBackendConfigResult, int, error) {
	if arg.URI == "" {
		return nil, nil, 0, errors.Errorf("empty uri not valid").Add(coreerrors.NotValid)
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, nil, 0, errors.Capture(err)
	}
	if uri.SourceUUID == "" {
		return nil, nil, 0, errors.Errorf("secret URI with empty source UUID not valid").Add(coreerrors.NotValid)
	}
	if arg.Revision == nil && !arg.Peek && !arg.Refresh {
		return nil, nil, 0, errors.Errorf("empty secret revision not valid").Add(coreerrors.NotValid)
	}

	crossModelRelationService, err := s.crossModelRelationServiceGetter(ctx, model.UUID(uri.SourceUUID))
	if err != nil {
		return nil, nil, 0, errors.Capture(err)
	}

	// The consuming application may hold several offer connections in this
	// model, each represented by its own synthetic application, so the
	// application token alone cannot identify the connection the secret
	// was granted over: the grant of the secret resolves the connection.
	grants, err := crossModelRelationService.GetRemoteConsumerSecretGrants(ctx, uri, coreapplication.UUID(arg.ApplicationToken))
	if errors.Is(err, secreterrors.SecretNotFound) {
		return nil, nil, 0, apiservererrors.ParamsErrorf(params.CodeNotFound, "secret %q not found", uri)
	} else if errors.Is(err, secreterrors.SecretAccessScopeNotFound) {
		// The consuming application holds no grant of the secret, so it
		// cannot be read through a cross model relation.
		return nil, nil, 0, apiservererrors.ErrPerm
	} else if err != nil {
		return nil, nil, 0, errors.Capture(err)
	}

	// Prefer the grant of the connection the macaroons identify, so the
	// relation key they carry is checked against the relation of that
	// connection. The grants are ordered by the name of the synthetic
	// application otherwise, so the choice is deterministic.
	grant := grants[0]
	if _, offerUUID, ok := crossmodelbakery.RelationInfoFromMacaroons(arg.Macaroons); ok {
		for _, g := range grants {
			if g.OfferUUID.String() == offerUUID {
				grant = g
				break
			}
		}
	}
	consumerApp := grant.ApplicationName
	consumerUnit, err := unit.NewNameFromParts(consumerApp, arg.UnitId)
	if err != nil {
		return nil, nil, 0, errors.Capture(err)
	}

	if err := s.checkRelationMacaroons(ctx, crossModelRelationService, consumerApp, arg.Macaroons, arg.BakeryVersion); err != nil {
		return nil, nil, 0, errors.Capture(err)
	}

	s.logger.Debugf(ctx, "consumer unit for application UUID %q: %v", arg.ApplicationToken, consumerUnit)

	val, valueRef, latestRevision, err := crossModelRelationService.ProcessRemoteConsumerGetSecret(ctx, uri, consumerUnit, arg.Revision, arg.Peek, arg.Refresh)
	switch {
	case errors.Is(err, secreterrors.PermissionDenied):
		return nil, nil, 0, apiservererrors.ErrPerm
	case errors.Is(err, secreterrors.SecretNotFound),
		errors.Is(err, secreterrors.SecretRevisionNotFound):
		if arg.Revision != nil {
			return nil, nil, 0, apiservererrors.ParamsErrorf(params.CodeNotFound, "revision %d for secret %q not found", *arg.Revision, uri)
		} else {
			return nil, nil, 0, apiservererrors.ParamsErrorf(params.CodeNotFound, "secret %q not found", uri)
		}
	}

	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, latestRevision, errors.Capture(err)
	}

	// Older controllers will not set the controller UUID in the arg, which means
	// that we assume a different controller for consume and offer models.
	// This breaks single controller microk8s cross model secrets, but not assuming
	// that breaks everything else.
	sameController := s.controllerUUID == arg.SourceControllerUUID
	backend, err := s.getBackend(ctx, model.UUID(uri.SourceUUID), sameController, content.ValueRef.BackendID, consumerUnit)
	return content, backend, latestRevision, errors.Capture(err)
}

func (s *CrossModelSecretsAPI) getBackend(ctx context.Context, modelUUID model.UUID, sameController bool, backendID string, consumer unit.Name) (*params.SecretBackendConfigResult, error) {
	secretService, err := s.secretServiceGetter(ctx, modelUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	cfgInfo, err := s.secretBackendService.BackendConfigInfo(ctx, secretbackendservice.BackendConfigParams{
		GrantedSecretsGetter: secretService.ListGrantedSecretsForBackend,
		Accessor: secret.SecretAccessor{
			Kind: secret.UnitAccessor,
			ID:   consumer.String(),
		},
		ModelUUID:      modelUUID,
		BackendIDs:     []string{backendID},
		SameController: sameController,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	for id, cfg := range cfgInfo.Configs {
		if id == backendID {
			return &params.SecretBackendConfigResult{
				ControllerUUID: cfg.ControllerUUID,
				ModelUUID:      cfg.ModelUUID,
				ModelName:      cfg.ModelName,
				Draining:       cfgInfo.ActiveID != backendID,
				Config: params.SecretBackendConfig{
					BackendType: cfg.BackendType,
					Params:      cfg.Config,
				},
			}, nil
		}
	}
	return nil, errors.Errorf("secret backend %q not found", backendID).Add(coreerrors.NotFound)
}
