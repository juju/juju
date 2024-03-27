// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.client.secrets")

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	authTag        names.Tag
	controllerUUID string
	modelUUID      string
	modelName      string

	activeBackendID string
	backends        map[string]provider.SecretsBackend

	secretService SecretService

	adminBackendConfigGetter               func(ctx context.Context) (*provider.ModelBackendConfigInfo, error)
	backendConfigGetterForUserSecretsWrite func(ctx context.Context, backendID string) (*provider.ModelBackendConfigInfo, error)
	backendGetter                          func(context.Context, *provider.ModelBackendConfig) (provider.SecretsBackend, error)
}

// SecretsAPIV1 is the backend for the Secrets facade v1.
type SecretsAPIV1 struct {
	*SecretsAPI
}

func (s *SecretsAPI) checkCanRead() error {
	return s.authorizer.HasPermission(permission.ReadAccess, names.NewModelTag(s.modelUUID))
}

func (s *SecretsAPI) checkCanWrite() error {
	return s.authorizer.HasPermission(permission.WriteAccess, names.NewModelTag(s.modelUUID))
}

func (s *SecretsAPI) checkCanAdmin() error {
	isAdmin, err := common.HasModelAdmin(s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if isAdmin {
		return nil
	}
	return apiservererrors.ErrPerm
}

// ListSecrets lists available secrets.
func (s *SecretsAPI) ListSecrets(ctx context.Context, arg params.ListSecretsArgs) (params.ListSecretResults, error) {
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
	var (
		err         error
		uri         *coresecrets.URI
		labels      domainsecret.Labels
		appOwners   domainsecret.ApplicationOwners
		unitOwners  domainsecret.UnitOwners
		modelOwners domainsecret.ModelOwners
		revisions   domainsecret.Revisions
	)
	if arg.Filter.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.Filter.URI)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
	}
	if arg.Filter.Label != nil {
		labels = append(labels, *arg.Filter.Label)
	}
	if arg.Filter.Revision != nil {
		revisions = append(revisions, *arg.Filter.Revision)
	}
	if arg.Filter.OwnerTag != nil {
		tag, err := names.ParseTag(*arg.Filter.OwnerTag)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		switch kind := tag.Kind(); kind {
		case names.ApplicationTagKind:
			appOwners = append(appOwners, tag.Id())
		case names.UnitTagKind:
			unitOwners = append(unitOwners, tag.Id())
		case names.ModelTagKind:
			modelOwners = append(modelOwners, tag.Id())
		default:
			return result, errors.NotValidf("secret owner tag kind %q", kind)
		}
	}
	metadata, revisionMetadata, err := s.secretService.ListSecrets(ctx, uri, revisions, labels, appOwners, unitOwners, modelOwners)
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
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
		grants, err := s.secretService.GetSecretGrants(ctx, m.URI, coresecrets.RoleView)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, g := range grants {
			secretResult.Access = append(secretResult.Access, params.AccessInfo{
				TargetTag: g.Target, ScopeTag: g.Scope, Role: g.Role,
			})
		}
		for _, r := range revisionMetadata[i] {
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
			val, err := s.secretContentFromBackend(ctx, m.URI, rev)
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

func (s *SecretsAPI) getBackendInfo(ctx context.Context) error {
	info, err := s.adminBackendConfigGetter(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for id, cfg := range info.Configs {
		s.backends[id], err = s.backendGetter(ctx, &cfg)
		if err != nil {
			return errors.Trace(err)
		}
	}
	s.activeBackendID = info.ActiveID
	return nil
}

// TODO(secrets) - rework once secret backend service lands
func (s *SecretsAPI) secretContentFromBackend(ctx context.Context, uri *coresecrets.URI, rev int) (coresecrets.SecretValue, error) {
	if s.activeBackendID == "" {
		err := s.getBackendInfo(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	lastBackendID := ""
	for {
		val, ref, err := s.secretService.GetSecretValue(ctx, uri, rev)
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
		val, err = backend.GetContent(ctx, ref.RevisionID)
		if err == nil || !errors.Is(err, errors.NotFound) || lastBackendID == backendID {
			return val, errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != s.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := s.getBackendInfo(ctx); initErr != nil {
			return nil, errors.Trace(initErr)
		}
		if s.activeBackendID == backendID {
			return nil, errors.Trace(err)
		}
	}
}

func (s *SecretsAPI) getBackendForUserSecretsWrite(ctx context.Context) (provider.SecretsBackend, error) {
	if s.activeBackendID == "" {
		if err := s.getBackendInfo(ctx); err != nil {
			return nil, errors.Trace(err)
		}
	}
	cfgInfo, err := s.backendConfigGetterForUserSecretsWrite(ctx, s.activeBackendID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[s.activeBackendID]
	if !ok {
		// This should never happen.
		return nil, errors.NotFoundf("secret backend %q", s.activeBackendID)
	}
	return s.backendGetter(ctx, &cfg)
}

// CreateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) CreateSecrets(_ context.Context, _ struct{}) {}

// CreateSecrets creates new secrets.
func (s *SecretsAPI) CreateSecrets(ctx context.Context, args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	backend, err := s.getBackendForUserSecretsWrite(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		id, err := s.createSecret(ctx, backend, arg)
		result.Results[i].Result = id
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsAPI) createSecret(ctx context.Context, backend provider.SecretsBackend, arg params.CreateSecretArg) (_ string, errOut error) {
	if arg.OwnerTag != "" && arg.OwnerTag != s.modelUUID {
		return "", errors.NotValidf("owner tag %q", arg.OwnerTag)
	}
	var uri *coresecrets.URI
	var err error
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
	revId, err := backend.SaveContent(ctx, uri, 1, coresecrets.NewSecretValue(arg.Content.Data))
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return "", errors.Trace(err)
	}
	if err == nil {
		defer func() {
			if errOut != nil {
				// If we failed to create the secret, we should delete the
				// secret value from the backend.
				if err2 := backend.DeleteContent(ctx, revId); err2 != nil &&
					!errors.Is(err2, errors.NotSupported) &&
					!errors.Is(err2, errors.NotFound) {
					logger.Errorf("failed to delete secret %q: %v", revId, err2)
				}
			}
		}()
		arg.Content.Data = nil
		arg.Content.ValueRef = &params.SecretValueRef{
			BackendID:  s.activeBackendID,
			RevisionID: revId,
		}
	}

	md, err := s.secretService.CreateSecret(ctx, uri, secretservice.CreateSecretParams{
		Version:            secrets.Version,
		ModelOwner:         &s.modelUUID,
		UpdateSecretParams: fromUpsertParams(nil, arg.UpsertSecretArg),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URI.String(), nil
}

func fromUpsertParams(autoPrune *bool, p params.UpsertSecretArg) secretservice.UpdateSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return secretservice.UpdateSecretParams{
		AutoPrune:   autoPrune,
		Description: p.Description,
		Label:       p.Label,
		Params:      p.Params,
		Data:        p.Content.Data,
		ValueRef:    valueRef,
	}
}

// UpdateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) UpdateSecrets(ctx context.Context, _ struct{}) {}

// UpdateSecrets creates new secrets.
func (s *SecretsAPI) UpdateSecrets(ctx context.Context, args params.UpdateUserSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	backend, err := s.getBackendForUserSecretsWrite(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, backend, arg)
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsAPI) updateSecret(ctx context.Context, backend provider.SecretsBackend, arg params.UpdateUserSecretArg) (errOut error) {
	if err := arg.Validate(); err != nil {
		return errors.Trace(err)
	}
	uri, err := s.secretURI(ctx, arg.URI, arg.ExistingLabel)
	if err != nil {
		return errors.Trace(err)
	}

	md, err := s.secretService.GetSecret(ctx, uri)
	if err != nil {
		// Check if the uri exists or not.
		return errors.Trace(err)
	}
	if len(arg.Content.Data) > 0 {
		revId, err := backend.SaveContent(ctx, uri, md.LatestRevision+1, coresecrets.NewSecretValue(arg.Content.Data))
		if err != nil && !errors.Is(err, errors.NotSupported) {
			return errors.Trace(err)
		}
		if err == nil {
			defer func() {
				if errOut != nil {
					// If we failed to update the secret, we should delete the
					// secret value from the backend for the new revision.
					if err2 := backend.DeleteContent(ctx, revId); err2 != nil &&
						!errors.Is(err2, errors.NotSupported) &&
						!errors.Is(err2, errors.NotFound) {
						logger.Errorf("failed to delete secret %q: %v", revId, err2)
					}
				}
			}()
			arg.Content.Data = nil
			arg.Content.ValueRef = &params.SecretValueRef{
				BackendID:  s.activeBackendID,
				RevisionID: revId,
			}
		}
	}
	_, err = s.secretService.UpdateSecret(ctx, uri, fromUpsertParams(arg.AutoPrune, arg.UpsertSecretArg))
	return errors.Trace(err)
}

func (s *SecretsAPI) secretURI(ctx context.Context, uriStr, label string) (*coresecrets.URI, error) {
	if uriStr == "" && label == "" {
		return nil, errors.New("must specify either URI or label")
	}
	if uriStr != "" {
		return coresecrets.ParseURI(uriStr)
	}
	md, err := s.secretService.GetUserSecretByLabel(ctx, label)
	if err != nil {
		return nil, errors.Annotatef(err, "getting user secret for label %q", label)
	}
	return md.URI, nil
}

// RemoveSecrets isn't on the v1 API.
func (s *SecretsAPIV1) RemoveSecrets(ctx context.Context, _ struct{}) {}

// RemoveSecrets remove user secret.
func (s *SecretsAPI) RemoveSecrets(ctx context.Context, args params.DeleteSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	if len(args.Args) == 0 {
		return result, nil
	}

	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Args {
		uri, err := s.secretURI(ctx, arg.URI, arg.Label)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = s.secretService.DeleteUserSecret(ctx, uri, arg.Revisions)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return result, nil
}

// GrantSecret isn't on the v1 API.
func (s *SecretsAPIV1) GrantSecret(ctx context.Context, _ struct{}) {}

// GrantSecret grants access to a user secret.
func (s *SecretsAPI) GrantSecret(ctx context.Context, arg params.GrantRevokeUserSecretArg) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(ctx, arg, s.secretService.GrantSecretAccess)
}

// RevokeSecret isn't on the v1 API.
func (s *SecretsAPIV1) RevokeSecret(ctx context.Context, _ struct{}) {}

// RevokeSecret revokes access to a user secret.
func (s *SecretsAPI) RevokeSecret(ctx context.Context, arg params.GrantRevokeUserSecretArg) (params.ErrorResults, error) {
	return s.secretsGrantRevoke(ctx, arg, s.secretService.RevokeSecretAccess)
}

type grantRevokeFunc func(context.Context, *coresecrets.URI, secretservice.SecretAccessParams) error

func (s *SecretsAPI) secretsGrantRevoke(ctx context.Context, arg params.GrantRevokeUserSecretArg, op grantRevokeFunc) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Applications)),
	}

	if arg.URI == "" && arg.Label == "" {
		return results, errors.New("must specify either URI or name")
	}

	if err := s.checkCanWrite(); err != nil {
		return results, errors.Trace(err)
	}

	uri, err := s.secretURI(ctx, arg.URI, arg.Label)
	if err != nil {
		return results, errors.Trace(err)
	}

	one := func(appName string) error {
		if err := op(ctx, uri, secretservice.SecretAccessParams{
			Scope:   permission.ID{ObjectType: permission.Model, Key: s.modelUUID},
			Subject: permission.ID{ObjectType: permission.Application, Key: appName},
			Role:    coresecrets.RoleView,
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
