// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.client.secrets")

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      string
	modelName      string

	activeBackendID string
	backends        map[string]provider.SecretsBackend

	secretsState    SecretsState
	secretsConsumer SecretsConsumer

	backendConfigGetter func() (*provider.ModelBackendConfigInfo, error)
	backendGetter       func(*provider.ModelBackendConfig) (provider.SecretsBackend, error)
}

// SecretsAPIV1 is the backend for the Secrets facade v1.
type SecretsAPIV1 struct {
	*SecretsAPI
}

func (s *SecretsAPI) checkCanRead() error {
	return s.authorizer.HasPermission(permission.ReadAccess, names.NewModelTag(s.modelUUID))
}

func (s *SecretsAPI) checkCanAdmin() error {
	_, err := common.HasModelAdmin(s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
	return err
}

// ListSecrets lists available secrets.
func (s *SecretsAPI) ListSecrets(arg params.ListSecretsArgs) (params.ListSecretResults, error) {
	result := params.ListSecretResults{}
	if arg.ShowSecrets {
		if err := s.checkCanAdmin(); err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if err := s.checkCanRead(); err != nil {
			return result, errors.Trace(err)
		}
	}
	var uri *coresecrets.URI
	if arg.Filter.URI != nil {
		var err error
		uri, err = coresecrets.ParseURI(*arg.Filter.URI)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
	}
	filter := state.SecretsFilter{
		URI: uri,
	}
	if arg.Filter.OwnerTag != nil {
		tag, err := names.ParseTag(*arg.Filter.OwnerTag)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		filter.OwnerTags = []names.Tag{tag}
	}
	metadata, err := s.secretsState.ListSecrets(filter)
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
	}
	revisionMetadata := make(map[string][]*coresecrets.SecretRevisionMetadata)
	for _, md := range metadata {
		if arg.Filter.Revision == nil {
			revs, err := s.secretsState.ListSecretRevisions(md.URI)
			if err != nil {
				return params.ListSecretResults{}, errors.Trace(err)
			}
			revisionMetadata[md.URI.ID] = revs
			continue
		}
		rev, err := s.secretsState.GetSecretRevision(md.URI, *arg.Filter.Revision)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		revisionMetadata[md.URI.ID] = []*coresecrets.SecretRevisionMetadata{rev}
	}
	result.Results = make([]params.ListSecretResult, len(metadata))
	for i, m := range metadata {
		secretResult := params.ListSecretResult{
			URI:              m.URI.String(),
			Version:          m.Version,
			OwnerTag:         m.OwnerTag,
			Description:      m.Description,
			Label:            m.Label,
			RotatePolicy:     string(m.RotatePolicy),
			NextRotateTime:   m.NextRotateTime,
			LatestRevision:   m.LatestRevision,
			LatestExpireTime: m.LatestExpireTime,
			CreateTime:       m.CreateTime,
			UpdateTime:       m.UpdateTime,
		}
		for _, r := range revisionMetadata[m.URI.ID] {
			backendName := r.BackendName
			if backendName == nil {
				if r.ValueRef != nil {
					if r.ValueRef.BackendID == s.modelUUID {
						name := kubernetes.BuiltInName(s.modelName)
						backendName = &name
					}
				} else {
					name := juju.BackendName
					backendName = &name
				}
			}
			secretResult.Revisions = append(secretResult.Revisions, params.SecretRevision{
				Revision:    r.Revision,
				CreateTime:  r.CreateTime,
				UpdateTime:  r.UpdateTime,
				ExpireTime:  r.ExpireTime,
				BackendName: backendName,
			})
		}
		if arg.ShowSecrets {
			rev := m.LatestRevision
			if arg.Filter.Revision != nil {
				rev = *arg.Filter.Revision
			}
			val, err := s.secretContentFromBackend(m.URI, rev)
			valueResult := &params.SecretValueResult{
				Error: apiservererrors.ServerError(err),
			}
			if err == nil {
				valueResult.Data = val.EncodedValues()
			}
			secretResult.Value = valueResult
		}
		result.Results[i] = secretResult
	}
	return result, nil
}

func (s *SecretsAPI) getBackendInfo() error {
	info, err := s.backendConfigGetter()
	if err != nil {
		return errors.Trace(err)
	}
	for id, cfg := range info.Configs {
		s.backends[id], err = s.backendGetter(&cfg)
		if err != nil {
			return errors.Trace(err)
		}
	}
	s.activeBackendID = info.ActiveID
	return nil
}

func (s *SecretsAPI) secretContentFromBackend(uri *coresecrets.URI, rev int) (coresecrets.SecretValue, error) {
	if s.activeBackendID == "" {
		err := s.getBackendInfo()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lastBackendID := ""
	for {
		val, ref, err := s.secretsState.GetSecretValue(uri, rev)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ref == nil {
			return val, nil
		}

		backendID := ref.BackendID
		backend, ok := s.backends[backendID]
		if !ok {
			return nil, errors.NotFoundf("external secret backend %q, have %q", backendID, s.backends)
		}
		val, err = backend.GetContent(context.TODO(), ref.RevisionID)
		if err == nil || !errors.Is(err, errors.NotFound) || lastBackendID == backendID {
			return val, errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != s.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := s.getBackendInfo(); initErr != nil {
			return nil, errors.Trace(initErr)
		}
		if s.activeBackendID == backendID {
			return nil, errors.Trace(err)
		}
	}
}

func (s *SecretsAPI) getActiveBackend() (provider.SecretsBackend, error) {
	if s.activeBackendID == "" {
		err := s.getBackendInfo()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	backend, ok := s.backends[s.activeBackendID]
	if !ok {
		return nil, errors.NotFoundf("external secret backend %q, have %q", s.activeBackendID, s.backends)
	}
	return backend, nil
}

// CreateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) CreateSecrets(_ struct{}) {}

// CreateSecrets creates new secrets.
func (s *SecretsAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	backend, err := s.getActiveBackend()
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		ID, err := s.createSecret(backend, arg)
		result.Results[i].Result = ID
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with label %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

func (s *SecretsAPI) createSecret(backend provider.SecretsBackend, arg params.CreateSecretArg) (_ string, err error) {
	if arg.OwnerTag != "" && arg.OwnerTag != s.modelUUID {
		return "", errors.NotValidf("owner tag %q", arg.OwnerTag)
	}
	secretOwner := names.NewModelTag(s.modelUUID)
	var uri *coresecrets.URI
	if arg.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.URI)
		if err != nil {
			return "", errors.Trace(err)
		}
	} else {
		uri = coresecrets.NewURI()
	}

	if len(arg.Content.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
	}
	revId, err := backend.SaveContent(context.TODO(), uri, 1, coresecrets.NewSecretValue(arg.Content.Data))
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return "", errors.Trace(err)
	}
	if err == nil {
		defer func() {
			if err != nil {
				// If we failed to create the secret, we should delete the
				// secret value from the backend.
				if err := backend.DeleteContent(context.TODO(), revId); err != nil &&
					!errors.Is(err, errors.NotSupported) &&
					!errors.Is(err, errors.NotFound) {
					logger.Errorf("failed to delete secret %q: %v", revId, err)
				}
			}
		}()
		arg.Content.Data = nil
		arg.Content.ValueRef = &params.SecretValueRef{
			BackendID:  s.activeBackendID,
			RevisionID: revId,
		}
	}

	md, err := s.secretsState.CreateSecret(uri, state.CreateSecretParams{
		Version:            secrets.Version,
		Owner:              secretOwner,
		UpdateSecretParams: fromUpsertParams(arg.UpsertSecretArg),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URI.String(), nil
}

func fromUpsertParams(p params.UpsertSecretArg) state.UpdateSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return state.UpdateSecretParams{
		LeaderToken: successfulToken{},
		Description: p.Description,
		Label:       p.Label,
		Params:      p.Params,
		Data:        p.Content.Data,
		ValueRef:    valueRef,
	}
}

// CreateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) GrantSecret(_ struct{}) {}

// GrantSecret grants access to a user secret.
func (s *SecretsAPI) GrantSecret(arg params.GrantRevokeUserSecretArg) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(arg, s.secretsConsumer.GrantSecretAccess)
}

// RevokeSecret isn't on the v1 API.
func (s *SecretsAPIV1) RevokeSecret(_ struct{}) {}

// RevokeSecret revokes access to a user secret.
func (s *SecretsAPI) RevokeSecret(arg params.GrantRevokeUserSecretArg) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(arg, s.secretsConsumer.RevokeSecretAccess)
}

type grantRevokeFunc func(*coresecrets.URI, state.SecretAccessParams) error

func (s *SecretsAPI) secretsGrantRevoke(arg params.GrantRevokeUserSecretArg, op grantRevokeFunc) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Applications)),
	}

	if err := s.checkCanAdmin(); err != nil {
		return results, errors.Trace(err)
	}

	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return results, errors.Trace(err)
	}
	scopeTag := names.NewModelTag(s.modelUUID)
	one := func(appName string) error {
		subjectTag := names.NewApplicationTag(appName)
		if err := op(uri, state.SecretAccessParams{
			LeaderToken: successfulToken{},
			Scope:       scopeTag,
			Subject:     subjectTag,
			Role:        coresecrets.RoleView,
		}); err != nil {
			return errors.Annotatef(err, "cannot change access to %q for %q", uri, appName)
		}
		return nil
	}
	for i, appName := range arg.Applications {
		results.Results[i].Error = apiservererrors.ServerError(one(appName))
	}
	return results, nil
}
