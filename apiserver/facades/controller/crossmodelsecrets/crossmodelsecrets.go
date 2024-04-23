// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	stdcontext "context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

type backendConfigGetter func(ctx stdcontext.Context, modelUUID string, sameController bool, backendID string, consumer names.Tag) (*provider.ModelBackendConfigInfo, error)
type secretServiceGetter func(modelUUID string) SecretService

// CrossModelSecretsAPI provides access to the CrossModelSecrets API facade.
type CrossModelSecretsAPI struct {
	resources facade.Resources

	mu             sync.Mutex
	authCtxt       *crossmodel.AuthContext
	controllerUUID string
	modelUUID      string

	secretServiceGetter secretServiceGetter
	backendConfigGetter backendConfigGetter
	crossModelState     CrossModelState
	stateBackend        StateBackend
	logger              loggo.Logger
}

// NewCrossModelSecretsAPI returns a new server-side CrossModelSecretsAPI facade.
func NewCrossModelSecretsAPI(
	resources facade.Resources,
	authContext *crossmodel.AuthContext,
	controllerUUID string,
	modelUUID string,
	secretServiceGetter secretServiceGetter,
	backendConfigGetter backendConfigGetter,
	crossModelState CrossModelState,
	stateBackend StateBackend,
	logger loggo.Logger,
) (*CrossModelSecretsAPI, error) {
	return &CrossModelSecretsAPI{
		resources:           resources,
		authCtxt:            authContext,
		controllerUUID:      controllerUUID,
		modelUUID:           modelUUID,
		secretServiceGetter: secretServiceGetter,
		backendConfigGetter: backendConfigGetter,
		crossModelState:     crossModelState,
		stateBackend:        stateBackend,
		logger:              logger,
	}, nil
}

// GetSecretAccessScope returns the tokens for the access scope of the specified secrets and consumers.
func (s *CrossModelSecretsAPI) GetSecretAccessScope(ctx stdcontext.Context, args params.GetRemoteSecretAccessArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		token, err := s.getSecretAccessScope(ctx, arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = token
	}
	return result, nil
}

func (s *CrossModelSecretsAPI) getSecretAccessScope(ctx stdcontext.Context, arg params.GetRemoteSecretAccessArg) (string, error) {
	if arg.URI == "" {
		return "", errors.NewNotValid(nil, "empty uri")
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return "", errors.Trace(err)
	}
	if uri.SourceUUID == "" {
		return "", errors.NotValidf("secret URI with empty source UUID")
	}

	consumerApp, err := s.crossModelState.GetRemoteApplicationTag(arg.ApplicationToken)
	if err != nil {
		return "", errors.Trace(err)
	}
	consumerUnit := names.NewUnitTag(fmt.Sprintf("%s/%d", consumerApp.Id(), arg.UnitId))

	s.logger.Debugf("consumer unit for token %q: %v", arg.ApplicationToken, consumerUnit.Id())

	secretService := s.secretServiceGetter(uri.SourceUUID)
	scopeTag, err := s.accessScope(ctx, secretService, uri, consumerUnit)
	if err != nil {
		return "", errors.Trace(err)
	}
	s.logger.Debugf("access scope for secret %v and consumer %v: %v", uri.String(), consumerUnit.Id(), scopeTag)
	return s.crossModelState.GetToken(scopeTag)
}

func (s *CrossModelSecretsAPI) checkRelationMacaroons(ctx stdcontext.Context, consumerTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check that the macaroon contains caveats for the relevant offer and
	// relation and that the consumer is in the relation.
	relKey, offerUUID, ok := crossmodel.RelationInfoFromMacaroons(mac)
	if !ok {
		s.logger.Debugf("missing relation or offer uuid from macaroons for consumer %v", consumerTag.Id())
		return apiservererrors.ErrPerm
	}
	valid, err := s.stateBackend.HasEndpoint(relKey, consumerTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !valid {
		s.logger.Debugf("secret consumer %q for relation %q not valid", consumerTag, relKey)
		return apiservererrors.ErrPerm
	}

	// A cross model secret can only be accessed if the corresponding cross model relation
	// it is scoped to is accessible by the supplied macaroon.
	auth := s.authCtxt.Authenticator()
	return auth.CheckRelationMacaroons(ctx, s.modelUUID, offerUUID, names.NewRelationTag(relKey), mac, version)
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPI) GetSecretContentInfo(ctx stdcontext.Context, args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
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

func (s *CrossModelSecretsAPI) getSecretContent(ctx stdcontext.Context, arg params.GetRemoteSecretContentArg) (*secrets.ContentParams, *params.SecretBackendConfigResult, int, error) {
	if arg.URI == "" {
		return nil, nil, 0, errors.NewNotValid(nil, "empty uri")
	}
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	}
	if uri.SourceUUID == "" {
		return nil, nil, 0, errors.NotValidf("secret URI with empty source UUID")
	}
	if arg.Revision == nil && !arg.Peek && !arg.Refresh {
		return nil, nil, 0, errors.NotValidf("empty secret revision")
	}

	appTag, err := s.crossModelState.GetRemoteApplicationTag(arg.ApplicationToken)
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	}
	consumer := names.NewUnitTag(fmt.Sprintf("%s/%d", appTag.Id(), arg.UnitId))

	if err := s.checkRelationMacaroons(ctx, appTag, arg.Macaroons, arg.BakeryVersion); err != nil {
		return nil, nil, 0, errors.Trace(err)
	}

	secretService := s.secretServiceGetter(uri.SourceUUID)

	if !s.canRead(ctx, secretService, uri, consumer) {
		return nil, nil, 0, apiservererrors.ErrPerm
	}

	var (
		wantRevision   int
		latestRevision int
	)
	// Use the latest revision as the current one if --peek.
	if arg.Peek || arg.Refresh {
		var err error
		latestRevision, err = secretService.UpdateRemoteConsumedRevision(ctx, uri, consumer.Id(), arg.Refresh)
		if err != nil {
			return nil, nil, 0, errors.Trace(err)
		}
		wantRevision = latestRevision
	} else {
		wantRevision = *arg.Revision
	}

	val, valueRef, err := secretService.GetSecretValue(ctx, uri, wantRevision)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, latestRevision, errors.Trace(err)
	}

	// Older controllers will not set the controller UUID in the arg, which means
	// that we assume a different controller for consume and offer models.
	// This breaks single controller microk8s cross model secrets, but not assuming
	// that breaks everything else.
	sameController := s.controllerUUID == arg.SourceControllerUUID
	backend, err := s.getBackend(ctx, uri.SourceUUID, sameController, content.ValueRef.BackendID, consumer)
	return content, backend, latestRevision, errors.Trace(err)
}

func (s *CrossModelSecretsAPI) getBackend(ctx stdcontext.Context, modelUUID string, sameController bool, backendID string, consumer names.Tag) (*params.SecretBackendConfigResult, error) {
	cfgInfo, err := s.backendConfigGetter(ctx, modelUUID, sameController, backendID, consumer)
	if err != nil {
		return nil, errors.Trace(err)
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
	return nil, errors.NotFoundf("secret backend %q", backendID)
}

// canRead returns true if the specified entity can read the secret.
func (s *CrossModelSecretsAPI) canRead(ctx stdcontext.Context, secretService SecretService, uri *coresecrets.URI, unit names.UnitTag) bool {
	s.logger.Debugf("check %s can read secret %s", unit, uri.ID)
	hasRole, _ := secretService.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   unit.Id(),
	})
	if hasRole.Allowed(coresecrets.RoleView) {
		return true
	}

	appName, _ := names.UnitApplication(unit.Id())
	kind := secretservice.ApplicationAccessor
	// Remote apps need a different accessor kind.
	if strings.HasPrefix(appName, "remote-") {
		kind = secretservice.RemoteApplicationAccessor
	}
	hasRole, _ = secretService.GetSecretAccess(ctx, uri, secretservice.SecretAccessor{
		Kind: kind,
		ID:   appName,
	})
	return hasRole.Allowed(coresecrets.RoleView)
}

func tagFromAccessScope(scope secretservice.SecretAccessScope) names.Tag {
	switch scope.Kind {
	case secretservice.ApplicationAccessScope:
		return names.NewApplicationTag(scope.ID)
	case secretservice.UnitAccessScope:
		return names.NewUnitTag(scope.ID)
	case secretservice.RelationAccessScope:
		return names.NewRelationTag(scope.ID)
	}
	return nil
}

func (s *CrossModelSecretsAPI) accessScope(ctx stdcontext.Context, secretService SecretService, uri *coresecrets.URI, unit names.UnitTag) (names.Tag, error) {
	s.logger.Debugf("scope for %q on secret %s", unit, uri.ID)
	scope, err := secretService.GetSecretAccessScope(ctx, uri, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   unit.Id(),
	})
	if err == nil || !errors.Is(err, secreterrors.SecretAccessScopeNotFound) {
		return tagFromAccessScope(scope), errors.Trace(err)
	}
	appName, _ := names.UnitApplication(unit.Id())
	kind := secretservice.ApplicationAccessor
	// Remote apps need a different accessor kind.
	if strings.HasPrefix(appName, "remote-") {
		kind = secretservice.RemoteApplicationAccessor
	}
	scope, err = secretService.GetSecretAccessScope(ctx, uri, secretservice.SecretAccessor{
		Kind: kind,
		ID:   appName,
	})
	return tagFromAccessScope(scope), errors.Trace(err)
}
