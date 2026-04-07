// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	commonmodel "github.com/juju/juju/apiserver/common/model"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
)

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	authTag        names.Tag
	controllerUUID string
	modelUUID      string
	modelName      string

<<<<<<< HEAD
	secretBackendService SecretBackendService
	secretService        SecretService
=======
	activeBackendID string
	backends        map[string]provider.SecretsBackend

	secretsState    SecretsState
	secretsConsumer SecretsConsumer

	adminBackendConfigGetter               func() (*provider.ModelBackendConfigInfo, error)
	backendConfigGetterForUserSecretsWrite func(backendID string, only []*coresecrets.URI) (*provider.ModelBackendConfigInfo, error)
	backendGetter                          func(*provider.ModelBackendConfig) (provider.SecretsBackend, error)
>>>>>>> 3.6
}

// SecretsAPIV1 is the backend for the Secrets facade v1.
type SecretsAPIV1 struct {
	*SecretsAPI
}

func (s *SecretsAPI) checkCanRead(ctx context.Context) error {
	return s.authorizer.HasPermission(ctx, permission.ReadAccess, names.NewModelTag(s.modelUUID))
}

func (s *SecretsAPI) checkCanWrite(ctx context.Context) error {
	return s.authorizer.HasPermission(ctx, permission.WriteAccess, names.NewModelTag(s.modelUUID))
}

func (s *SecretsAPI) checkCanAdmin(ctx context.Context) error {
	isAdmin, err := commonmodel.HasModelAdmin(ctx, s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
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
		if err := s.checkCanAdmin(ctx); err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if err := s.checkCanRead(ctx); err != nil {
			return result, errors.Trace(err)
		}
	}
<<<<<<< HEAD
	var (
		err    error
		uri    *coresecrets.URI
		labels domainsecret.Labels
		owner  *domainsecret.CharmSecretOwner
	)
	if arg.Filter.URI != nil {
		uri, err = coresecrets.ParseURI(*arg.Filter.URI)
=======
	filter := state.SecretsFilter{}
	if arg.Filter.URI != nil {
		uri, err := coresecrets.ParseURI(*arg.Filter.URI)
>>>>>>> 3.6
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		filter.URIs = append(filter.URIs, uri)
	}
	if arg.Filter.Label != nil {
<<<<<<< HEAD
		labels = append(labels, *arg.Filter.Label)
=======
		filter.Labels = append(filter.Labels, *arg.Filter.Label)
>>>>>>> 3.6
	}
	if arg.Filter.OwnerTag != nil {
		tag, err := names.ParseTag(*arg.Filter.OwnerTag)
		if err != nil {
			return params.ListSecretResults{}, errors.Trace(err)
		}
		switch kind := tag.Kind(); kind {
		case names.ApplicationTagKind:
			owner = &domainsecret.CharmSecretOwner{
				Kind: domainsecret.ApplicationCharmSecretOwner,
				ID:   tag.Id(),
			}
		case names.UnitTagKind:
			owner = &domainsecret.CharmSecretOwner{
				Kind: domainsecret.UnitCharmSecretOwner,
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
			URI:                    m.URI.String(),
			Version:                m.Version,
			OwnerTag:               ownerTag.String(),
			Description:            m.Description,
			Label:                  m.Label,
			RotatePolicy:           string(m.RotatePolicy),
			NextRotateTime:         m.NextRotateTime,
			LatestRevision:         m.LatestRevision,
			LatestRevisionChecksum: m.LatestRevisionChecksum,
			LatestExpireTime:       m.LatestExpireTime,
			CreateTime:             m.CreateTime,
			UpdateTime:             m.UpdateTime,
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
			// We want to maintain the behavior that if the backend is kubernetes,
			// we return the built-in name, even though the backend name is now populated.
			// Backend name should always be non-nil here.
			backendName := r.BackendName
			if backendName == nil {
				return params.ListSecretResults{}, errors.New("retrieving secret revision backend name for secret " + m.Label)
			}
			if *r.BackendName == kubernetes.BackendName {
				name := kubernetes.BuiltInName(s.modelName)
				backendName = &name

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

func tagFromSubject(access domainsecret.SecretAccessor) (names.Tag, error) {
	switch kind := access.Kind; kind {
	case domainsecret.UnitAccessor:
		return names.NewUnitTag(access.ID), nil
	case domainsecret.ApplicationAccessor:
		return names.NewApplicationTag(access.ID), nil
	case domainsecret.ModelAccessor:
		return names.NewModelTag(access.ID), nil
	default:
		return nil, errors.NotValidf("subject kind %q", kind)
	}
}

<<<<<<< HEAD
func tagFromAccessScope(access domainsecret.SecretAccessScope) (names.Tag, error) {
	switch kind := access.Kind; kind {
	case domainsecret.UnitAccessScope:
		return names.NewUnitTag(access.ID), nil
	case domainsecret.ApplicationAccessScope:
		return names.NewApplicationTag(access.ID), nil
	case domainsecret.ModelAccessScope:
		return names.NewModelTag(access.ID), nil
	case domainsecret.RelationAccessScope:
		return names.NewRelationTag(access.ID), nil
	default:
		return nil, errors.NotValidf("access scope kind %q", kind)
	}
=======
// getBackendForUserSecretsWrite returns the secret backend for user secrets,
// optionally limited to the list of secrets if a non-zero number is supplied.
func (s *SecretsAPI) getBackendForUserSecretsWrite(
	only []*coresecrets.URI,
) (provider.SecretsBackend, error) {
	if s.activeBackendID == "" {
		if err := s.getBackendInfo(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	cfgInfo, err := s.backendConfigGetterForUserSecretsWrite(
		s.activeBackendID, only)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[s.activeBackendID]
	if !ok {
		// This should never happen.
		return nil, errors.NotFoundf("secret backend %q", s.activeBackendID)
	}
	return s.backendGetter(&cfg)
>>>>>>> 3.6
}

// CreateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) CreateSecrets(_ context.Context, _ struct{}) {}

// CreateSecrets creates new secrets.
func (s *SecretsAPI) CreateSecrets(ctx context.Context, args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
<<<<<<< HEAD
	if err := s.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		id, err := s.createSecret(ctx, arg)
		result.Results[i].Result = id
		if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
=======
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}

	// Validate secrets before generating a secret URI.
	for i, arg := range args.Args {
		var err error
		if arg.URI != nil {
			err = errors.NotValidf(
				"secret uri cannot be set on user secret create",
			)
		}
		if arg.OwnerTag != "" && arg.OwnerTag != s.modelUUID {
			err = errors.NotValidf("owner tag %q", arg.OwnerTag)
		}
		if len(arg.Content.Data) == 0 {
			err = errors.NotValidf("empty secret value")
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}

	secretOwner := names.NewModelTag(s.modelUUID)
	uris := make([]*coresecrets.URI, len(args.Args))
	// Generate secret URIs
	for i := range args.Args {
		if result.Results[i].Error != nil {
			continue
		}

		uri := coresecrets.NewURI()
		err := s.secretsState.ReserveSecret(uri, secretOwner)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		uris[i] = uri
	}

	backend, err := s.getBackendForUserSecretsWrite(uris)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		if result.Results[i].Error != nil {
			continue
		}
		var err error
		result.Results[i].Result, err = s.createSecret(backend, arg, uris[i])
		if errors.Is(err, state.LabelExists) {
>>>>>>> 3.6
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

<<<<<<< HEAD
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
=======
type successfulToken struct{}

// Check implements lease.Token.
func (t successfulToken) Check() error {
	return nil
}

func (s *SecretsAPI) createSecret(
	backend provider.SecretsBackend,
	arg params.CreateSecretArg,
	uri *coresecrets.URI,
) (_ string, errOut error) {
	secretOwner := names.NewModelTag(s.modelUUID)
>>>>>>> 3.6

	v := coresecrets.NewSecretValue(arg.Content.Data)
	checksum, err := v.Checksum()
	if err != nil {
		return "", errors.Annotate(err, "calculating secret checksum")
	}
	arg.UpsertSecretArg.Content.Checksum = checksum
<<<<<<< HEAD
	err = s.secretService.CreateUserSecret(ctx, uri, secretservice.CreateUserSecretParams{
		Version:                secrets.Version,
		UpdateUserSecretParams: fromUpsertParams(s.modelUUID, nil, arg.UpsertSecretArg),
=======

	revId, err := backend.SaveContent(context.TODO(), uri, 1, coresecrets.NewSecretValue(arg.Content.Data))
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return "", errors.Trace(err)
	} else if err == nil {
		defer func() {
			if errOut != nil {
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
		UpdateSecretParams: fromUpsertParams(nil, arg.UpsertSecretArg),
>>>>>>> 3.6
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return uri.String(), nil
}

func fromUpsertParams(modelUUID string, autoPrune *bool, p params.UpsertSecretArg) secretservice.UpdateUserSecretParams {
	return secretservice.UpdateUserSecretParams{
		Accessor:    domainsecret.SecretAccessor{Kind: domainsecret.ModelAccessor, ID: modelUUID},
		AutoPrune:   autoPrune,
		Description: p.Description,
		Label:       p.Label,
		Params:      p.Params,
		Data:        p.Content.Data,
		Checksum:    p.Content.Checksum,
	}
}

// UpdateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) UpdateSecrets(ctx context.Context, _ struct{}) {}

// UpdateSecrets updates user secrets.
func (s *SecretsAPI) UpdateSecrets(ctx context.Context, args params.UpdateUserSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
<<<<<<< HEAD
	if err := s.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateSecret(ctx, arg)
		if errors.Is(err, secreterrors.SecretLabelAlreadyExists) {
=======
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}

	uris := make([]*coresecrets.URI, len(args.Args))
	for i, arg := range args.Args {
		var err error
		if arg.URI != "" {
			uris[i], err = coresecrets.ParseURI(arg.URI)
		} else {
			uris[i], err = s.getSecretURI(s.modelUUID, arg.ExistingLabel)
		}
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
		}
	}

	backend, err := s.getBackendForUserSecretsWrite(uris)
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateSecret(backend, arg, uris[i])
		if errors.Is(err, state.LabelExists) {
>>>>>>> 3.6
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

<<<<<<< HEAD
func (s *SecretsAPI) updateSecret(ctx context.Context, arg params.UpdateUserSecretArg) (errOut error) {
	if err := arg.Validate(); err != nil {
		return errors.Trace(err)
	}
	uri, err := s.secretURI(ctx, arg.URI, arg.ExistingLabel)
	if err != nil {
=======
func (s *SecretsAPI) updateSecret(
	backend provider.SecretsBackend,
	arg params.UpdateUserSecretArg,
	uri *coresecrets.URI,
) (errOut error) {
	if err := arg.Validate(); err != nil {
		return errors.Trace(err)
	}

	md, err := s.secretsState.GetSecret(uri)
	if err != nil {
		// Check if the uri exists or not.
>>>>>>> 3.6
		return errors.Trace(err)
	}
	if len(arg.Content.Data) > 0 {
		v := coresecrets.NewSecretValue(arg.Content.Data)
		checksum, err := v.Checksum()
		if err != nil {
			return errors.Annotate(err, "calculating secret checksum")
		}
		arg.Content.Checksum = checksum
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

	if err := s.checkCanWrite(ctx); err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Args {
		uri, err := s.secretURI(ctx, arg.URI, arg.Label)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = s.secretService.DeleteSecret(ctx, uri, domainsecret.DeleteSecretParams{
			Accessor:  domainsecret.SecretAccessor{Kind: domainsecret.ModelAccessor, ID: s.modelUUID},
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

type grantRevokeFunc func(context.Context, *coresecrets.URI, domainsecret.SecretAccessParams) error

<<<<<<< HEAD
func (s *SecretsAPI) secretsGrantRevoke(ctx context.Context, arg params.GrantRevokeUserSecretArg, op grantRevokeFunc) (params.ErrorResults, error) {
=======
func (s *SecretsAPI) getSecretURI(modelUUID, label string) (*coresecrets.URI, error) {
	results, err := s.secretsState.ListSecrets(state.SecretsFilter{
		Labels:    []string{label},
		OwnerTags: []names.Tag{names.NewModelTag(modelUUID)},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results) == 0 {
		return nil, errors.NotFoundf("secret %q", label)
	}
	if len(results) > 1 {
		return nil, errors.NotFoundf("more than 1 secret with label %q", label)
	}
	return results[0].URI, nil
}

func (s *SecretsAPI) secretsGrantRevoke(arg params.GrantRevokeUserSecretArg, op grantRevokeFunc) (params.ErrorResults, error) {
>>>>>>> 3.6
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Applications)),
	}

	if arg.URI == "" && arg.Label == "" {
		return results, errors.New("must specify either URI or name")
	}

	if err := s.checkCanWrite(ctx); err != nil {
		return results, errors.Trace(err)
	}

	uri, err := s.secretURI(ctx, arg.URI, arg.Label)
	if err != nil {
		return results, errors.Trace(err)
	}

	one := func(appName string) error {
		if err := op(ctx, uri, domainsecret.SecretAccessParams{
			Accessor: domainsecret.SecretAccessor{Kind: domainsecret.ModelAccessor, ID: s.modelUUID},
			Scope:    domainsecret.SecretAccessScope{Kind: domainsecret.ModelAccessScope, ID: s.modelUUID},
			Subject:  domainsecret.SecretAccessor{Kind: domainsecret.ApplicationAccessor, ID: appName},
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
