// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	stdcontext "context"
	"fmt"
	"sync"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
)

type backendConfigGetter func(modelUUID string, sameController bool, backendID string, consumer names.Tag) (*provider.ModelBackendConfigInfo, error)
type secretStateGetter func(modelUUID string) (SecretsState, SecretsConsumer, func() bool, error)

// CrossModelSecretsAPI provides access to the CrossModelSecrets API facade.
type CrossModelSecretsAPI struct {
	resources facade.Resources

	ctx            stdcontext.Context
	mu             sync.Mutex
	authCtxt       *crossmodel.AuthContext
	controllerUUID string
	modelUUID      string

	secretsStateGetter  secretStateGetter
	backendConfigGetter backendConfigGetter
	crossModelState     CrossModelState
	stateBackend        StateBackend
}

// NewCrossModelSecretsAPI returns a new server-side CrossModelSecretsAPI facade.
func NewCrossModelSecretsAPI(
	resources facade.Resources,
	authContext *crossmodel.AuthContext,
	controllerUUID string,
	modelUUID string,
	secretsStateGetter secretStateGetter,
	backendConfigGetter backendConfigGetter,
	crossModelState CrossModelState,
	stateBackend StateBackend,
) (*CrossModelSecretsAPI, error) {
	return &CrossModelSecretsAPI{
		ctx:                 stdcontext.Background(),
		resources:           resources,
		authCtxt:            authContext,
		controllerUUID:      controllerUUID,
		modelUUID:           modelUUID,
		secretsStateGetter:  secretsStateGetter,
		backendConfigGetter: backendConfigGetter,
		crossModelState:     crossModelState,
		stateBackend:        stateBackend,
	}, nil
}

var logger = loggo.GetLoggerWithLabels("juju.apiserver.crossmodelsecrets", corelogger.SECRETS)

// GetSecretAccessScope returns the tokens for the access scope of the specified secrets and consumers.
func (s *CrossModelSecretsAPI) GetSecretAccessScope(args params.GetRemoteSecretAccessArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		token, err := s.getSecretAccessScope(arg)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = token
	}
	return result, nil
}

func (s *CrossModelSecretsAPI) getSecretAccessScope(arg params.GetRemoteSecretAccessArg) (string, error) {
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

	logger.Debugf("consumer unit for token %q: %v", arg.ApplicationToken, consumerUnit.Id())

	_, secretsConsumer, closer, err := s.secretsStateGetter(uri.SourceUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer closer()
	scopeTag, err := s.accessScope(secretsConsumer, uri, consumerUnit)
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Debugf("access scope for secret %v and consumer %v: %v", uri.String(), consumerUnit.Id(), scopeTag)
	return s.crossModelState.GetToken(scopeTag)
}

func (s *CrossModelSecretsAPI) checkRelationMacaroons(consumerTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check that the macaroon contains caveats for the relevant offer and
	// relation and that the consumer is in the relation.
	relKey, offerUUID, ok := crossmodel.RelationInfoFromMacaroons(mac)
	if !ok {
		logger.Debugf("missing relation or offer uuid from macaroons for consumer %v", consumerTag.Id())
		return apiservererrors.ErrPerm
	}
	valid, err := s.stateBackend.HasEndpoint(relKey, consumerTag.Id())
	if err != nil {
		return errors.Trace(err)
	}
	if !valid {
		logger.Debugf("secret consumer %q for relation %q not valid", consumerTag, relKey)
		return apiservererrors.ErrPerm
	}

	// A cross model secret can only be accessed if the corresponding cross model relation
	// it is scoped to is accessible by the supplied macaroon.
	auth := s.authCtxt.Authenticator()
	return auth.CheckRelationMacaroons(s.ctx, s.modelUUID, offerUUID, names.NewRelationTag(relKey), mac, version)
}

// GetSecretContentInfo returns the secret values for the specified secrets.
func (s *CrossModelSecretsAPI) GetSecretContentInfo(args params.GetRemoteSecretContentArgs) (params.SecretContentResults, error) {
	result := params.SecretContentResults{
		Results: make([]params.SecretContentResult, len(args.Args)),
	}
	for i, arg := range args.Args {
		content, backend, latestRevision, err := s.getSecretContent(arg)
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

func (s *CrossModelSecretsAPI) getSecretContent(arg params.GetRemoteSecretContentArg) (*secrets.ContentParams, *params.SecretBackendConfigResult, int, error) {
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

	if err := s.checkRelationMacaroons(appTag, arg.Macaroons, arg.BakeryVersion); err != nil {
		return nil, nil, 0, errors.Trace(err)
	}

	secretState, secretsConsumer, closer, err := s.secretsStateGetter(uri.SourceUUID)
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	}
	defer closer()

	if !s.canRead(secretsConsumer, uri, consumer) {
		return nil, nil, 0, apiservererrors.ErrPerm
	}

	var (
		wantRevision   int
		latestRevision int
	)
	// Use the latest revision as the current one if --peek.
	if arg.Peek || arg.Refresh {
		var err error
		latestRevision, err = s.updateConsumedRevision(secretState, secretsConsumer, consumer, uri, arg.Refresh)
		if err != nil {
			return nil, nil, 0, errors.Trace(err)
		}
		wantRevision = latestRevision
	} else {
		wantRevision = *arg.Revision
	}

	val, valueRef, err := secretState.GetSecretValue(uri, wantRevision)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, latestRevision, errors.Trace(err)
	}

	// Older controllers will not set the controller UUID in the arg, which means
	// that we assume a different controller for consume and offer models.
	// This breaks single controller microk8s cross model secrets, but not assuming
	// that breaks everything else.
	sameController := s.controllerUUID == arg.SourceControllerUUID
	backend, err := s.getBackend(uri.SourceUUID, sameController, content.ValueRef.BackendID, consumer)
	return content, backend, latestRevision, errors.Trace(err)
}

func (s *CrossModelSecretsAPI) updateConsumedRevision(secretsState SecretsState, secretsConsumer SecretsConsumer, consumer names.Tag, uri *coresecrets.URI, refresh bool) (int, error) {
	consumerInfo, err := secretsConsumer.GetSecretRemoteConsumer(uri, consumer)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return 0, errors.Trace(err)
	}
	refresh = refresh ||
		err != nil // Not found, so need to create one.

	md, err := secretsState.GetSecret(uri)
	if err != nil {
		return 0, errors.Trace(err)
	}

	if refresh {
		if consumerInfo == nil {
			consumerInfo = &coresecrets.SecretConsumerMetadata{}
		}
		consumerInfo.LatestRevision = md.LatestRevision
		consumerInfo.CurrentRevision = md.LatestRevision
		if err := secretsConsumer.SaveSecretRemoteConsumer(uri, consumer, consumerInfo); err != nil {
			return 0, errors.Trace(err)
		}
	}
	return md.LatestRevision, nil
}

func (s *CrossModelSecretsAPI) getBackend(modelUUID string, sameController bool, backendID string, consumer names.Tag) (*params.SecretBackendConfigResult, error) {
	cfgInfo, err := s.backendConfigGetter(modelUUID, sameController, backendID, consumer)
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
func (s *CrossModelSecretsAPI) canRead(secretsConsumer SecretsConsumer, uri *coresecrets.URI, entity names.Tag) bool {
	logger.Debugf("check %s can read secret %s", entity, uri.ID)
	hasRole, _ := secretsConsumer.SecretAccess(uri, entity)
	if hasRole.Allowed(coresecrets.RoleView) {
		return true
	}

	// Unit access not granted, see if app access is granted.
	if entity.Kind() != names.UnitTagKind {
		return false
	}
	appName, _ := names.UnitApplication(entity.Id())
	hasRole, _ = secretsConsumer.SecretAccess(uri, names.NewApplicationTag(appName))
	return hasRole.Allowed(coresecrets.RoleView)
}

func (s *CrossModelSecretsAPI) accessScope(secretsConsumer SecretsConsumer, uri *coresecrets.URI, entity names.Tag) (names.Tag, error) {
	logger.Debugf("scope for %q on secret %s", entity, uri.ID)
	scope, err := secretsConsumer.SecretAccessScope(uri, entity)
	if err == nil || !errors.IsNotFound(err) || entity.Kind() != names.UnitTagKind {
		return scope, errors.Trace(err)
	}
	appName, _ := names.UnitApplication(entity.Id())
	return secretsConsumer.SecretAccessScope(uri, names.NewApplicationTag(appName))
}
