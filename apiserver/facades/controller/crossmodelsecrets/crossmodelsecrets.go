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

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
)

type backendConfigGetter func(string) (*provider.ModelBackendConfigInfo, error)
type secretStateGetter func(modelUUID string) (SecretsState, SecretsConsumer, func() bool, error)

// CrossModelSecretsAPI provides access to the CrossModelSecrets API facade.
type CrossModelSecretsAPI struct {
	resources facade.Resources

	ctx       stdcontext.Context
	mu        sync.Mutex
	modelUUID string

	secretsStateGetter  secretStateGetter
	backendConfigGetter backendConfigGetter
	crossModelState     CrossModelState
}

// NewCrossModelSecretsAPI returns a new server-side CrossModelSecretsAPI facade.
func NewCrossModelSecretsAPI(
	resources facade.Resources,
	modelUUID string,
	secretsStateGetter secretStateGetter,
	backendConfigGetter backendConfigGetter,
	crossModelState CrossModelState,
) (*CrossModelSecretsAPI, error) {
	return &CrossModelSecretsAPI{
		ctx:                 stdcontext.Background(),
		resources:           resources,
		modelUUID:           modelUUID,
		secretsStateGetter:  secretsStateGetter,
		backendConfigGetter: backendConfigGetter,
		crossModelState:     crossModelState,
	}, nil
}

var logger = loggo.GetLogger("juju.apiserver.crossmodelsecrets")

func (s *CrossModelSecretsAPI) checkMacaroonsForConsumer(consumerTag names.Tag, secretURI string, mac macaroon.Slice, version bakery.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO(cmr secrets)
	return nil
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
	if arg.Revision == nil && !arg.Latest {
		return nil, nil, 0, errors.NotValidf("empty secret revision")
	}

	app, err := s.crossModelState.GetRemoteEntity(arg.ApplicationToken)
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	}
	consumer := names.NewUnitTag(fmt.Sprintf("%s/%d", app.Id(), arg.UnitId))

	if err := s.checkMacaroonsForConsumer(app, uri.ID, arg.Macaroons, arg.BakeryVersion); err != nil {
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

	md, err := secretState.GetSecret(uri)
	if err != nil {
		return nil, nil, 0, errors.Trace(err)
	}

	var wantRevision int
	// Use the latest revision as the current one if --peek.
	if arg.Latest {
		wantRevision = md.LatestRevision
	} else {
		wantRevision = *arg.Revision
	}

	val, valueRef, err := secretState.GetSecretValue(uri, wantRevision)
	content := &secrets.ContentParams{SecretValue: val, ValueRef: valueRef}
	if err != nil || content.ValueRef == nil {
		return content, nil, md.LatestRevision, errors.Trace(err)
	}
	backend, err := s.getBackend(uri.SourceUUID, content.ValueRef.BackendID)
	return content, backend, md.LatestRevision, errors.Trace(err)
}

func (s *CrossModelSecretsAPI) getBackend(modelUUID string, backendID string) (*params.SecretBackendConfigResult, error) {
	cfgInfo, err := s.backendConfigGetter(modelUUID)
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
