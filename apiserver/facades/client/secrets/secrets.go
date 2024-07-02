// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	authTag        names.Tag
	controllerUUID string
	modelUUID      string
	modelName      string

	secretBackendService SecretBackendService
	secretService        SecretService
}

// SecretsAPIV1 is the backend for the Secrets facade v1.
type SecretsAPIV1 struct {
	*SecretsAPIV2
}

// SecretsAPIV2 is the backend for the Secrets facade v2.
type SecretsAPIV2 struct {
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
// If args specifies secret owners, then only charm secrets are queried because user secret don't have owners as such.
// If no owners are specified, we use the more generic list method when returns all types of secret.
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
		err    error
		uri    *coresecrets.URI
		labels domainsecret.Labels
		owner  *secretservice.CharmSecretOwner
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
	if arg.Filter.OwnerTag != nil {
		tag, err := names.ParseTag(*arg.Filter.OwnerTag)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		switch kind := tag.Kind(); kind {
		case names.ApplicationTagKind:
			owner = &secretservice.CharmSecretOwner{
				Kind: secretservice.ApplicationOwner,
				ID:   tag.Id(),
			}
		case names.UnitTagKind:
			owner = &secretservice.CharmSecretOwner{
				Kind: secretservice.UnitOwner,
				ID:   tag.Id(),
			}
		default:
			return result, errors.NotValidf("secret owner tag kind %q", kind)
		}
	}
	var (
		metadata         []*coresecrets.SecretMetadata
		revisionMetadata [][]*coresecrets.SecretRevisionMetadata
	)
	if owner == nil {
		metadata, revisionMetadata, err = s.secretService.ListSecrets(ctx, uri, arg.Filter.Revision, labels)
	} else {
		metadata, revisionMetadata, err = s.secretService.ListCharmSecrets(ctx, *owner)
	}
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
	}
	result.Results = make([]params.ListSecretResult, len(metadata))
	for i, m := range metadata {
		ownerTag, err := commonsecrets.OwnerTagFromOwner(m.Owner)
		if err != nil {
			// This should never happen.
			return params.ListSecretResults{}, errors.Trace(err)
		}
		secretResult := params.ListSecretResult{
			URI:              m.URI.String(),
			Version:          m.Version,
			OwnerTag:         ownerTag.String(),
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
			val, err := s.secretService.GetSecretContentFromBackend(ctx, m.URI, rev)
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
	for i, arg := range args.Args {
		id, err := s.createSecret(ctx, arg)
		result.Results[i].Result = id
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (_ string, errOut error) {
	if arg.OwnerTag != "" && arg.OwnerTag != s.modelUUID {
		return "", errors.NotValidf("owner tag %q", arg.OwnerTag)
	}
	if len(arg.Content.Data) == 0 {
		return "", errors.NotValidf("empty secret value")
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

	err = s.secretService.CreateUserSecret(ctx, uri, secretservice.CreateUserSecretParams{
		Version:                secrets.Version,
		UpdateUserSecretParams: fromUpsertParams(s.modelUUID, nil, arg.UpsertSecretArg),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return uri.String(), nil
}

func fromUpsertParams(modelUUID string, autoPrune *bool, p params.UpsertSecretArg) secretservice.UpdateUserSecretParams {
	return secretservice.UpdateUserSecretParams{
		Accessor:    secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: modelUUID},
		AutoPrune:   autoPrune,
		Description: p.Description,
		Label:       p.Label,
		Params:      p.Params,
		Data:        p.Content.Data,
	}
}

// UpdateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) UpdateSecrets(ctx context.Context, _ struct{}) {}

// UpdateSecrets updates user secrets.
func (s *SecretsAPI) UpdateSecrets(ctx context.Context, args params.UpdateUserSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg)
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsAPI) updateSecret(ctx context.Context, arg params.UpdateUserSecretArg) (errOut error) {
	if err := arg.Validate(); err != nil {
		return errors.Trace(err)
	}
	uri, err := s.secretURI(ctx, arg.URI, arg.ExistingLabel)
	if err != nil {
		return errors.Trace(err)
	}

	err = s.secretService.UpdateUserSecret(ctx, uri, fromUpsertParams(s.modelUUID, arg.AutoPrune, arg.UpsertSecretArg))
	return errors.Trace(err)
}

func (s *SecretsAPI) secretURI(ctx context.Context, uriStr, label string) (*coresecrets.URI, error) {
	if uriStr == "" && label == "" {
		return nil, errors.New("must specify either URI or label")
	}
	if uriStr != "" {
		return coresecrets.ParseURI(uriStr)
	}
	uri, err := s.secretService.GetUserSecretURIByLabel(ctx, label)
	if err != nil {
		return nil, errors.Annotatef(err, "getting user secret for label %q", label)
	}
	return uri, nil
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
		err = s.secretService.DeleteSecret(ctx, uri, secretservice.DeleteSecretParams{
			Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: s.modelUUID},
			Revisions: arg.Revisions,
		})
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
			Accessor: secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: s.modelUUID},
			Scope:    secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: s.modelUUID},
			Subject:  secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: appName},
			Role:     coresecrets.RoleView,
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

// GetModelSecretBackend isn't implemented in the SecretsAPIV2 facade.
func (s *SecretsAPIV2) GetModelSecretBackend(context.Context) {}

// GetModelSecretBackend returns the secret backend for the model.
func (s *SecretsAPI) GetModelSecretBackend(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}
	if err := s.authorizer.HasPermission(permission.ReadAccess, names.NewModelTag(s.modelUUID)); err != nil {
		return result, errors.Trace(err)
	}

	name, err := s.secretBackendService.GetModelSecretBackend(ctx, coremodel.UUID(s.modelUUID))
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		result.Result = name
	}
	return result, nil
}

// SetModelSecretBackend isn't implemented in the SecretsAPIV2 facade.
func (s *SecretsAPIV2) SetModelSecretBackend(_ context.Context, _ struct{}) {}

// SetModelSecretBackend sets the secret backend name for the model.
func (s *SecretsAPI) SetModelSecretBackend(ctx context.Context, arg params.SetModelSecretBackendArg) (params.ErrorResult, error) {
	if err := s.authorizer.HasPermission(permission.WriteAccess, names.NewModelTag(s.modelUUID)); err != nil {
		return params.ErrorResult{}, errors.Trace(err)
	}
	err := s.secretBackendService.SetModelSecretBackend(ctx, coremodel.UUID(s.modelUUID), arg.SecretBackendName)
	return params.ErrorResult{Error: apiservererrors.ServerError(err)}, nil
}
