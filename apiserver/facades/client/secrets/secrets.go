// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
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
	authTag        names.Tag
	controllerUUID string
	modelUUID      string
	modelName      string

	activeBackendID string
	backends        map[string]provider.SecretsBackend

	secretsState    SecretsState
	secretsConsumer SecretsConsumer

	adminBackendConfigGetter               func() (*provider.ModelBackendConfigInfo, error)
	backendConfigGetterForUserSecretsWrite func(backendID string) (*provider.ModelBackendConfigInfo, error)
	backendGetter                          func(*provider.ModelBackendConfig) (provider.SecretsBackend, error)
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
		URI:   uri,
		Label: arg.Filter.Label,
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
			URI:                    m.URI.String(),
			Version:                m.Version,
			OwnerTag:               m.OwnerTag,
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
		grants, err := s.secretsState.SecretGrants(m.URI, coresecrets.RoleView)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, g := range grants {
			secretResult.Access = append(secretResult.Access, params.AccessInfo{
				TargetTag: g.Target, ScopeTag: g.Scope, Role: g.Role,
			})
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
	info, err := s.adminBackendConfigGetter()
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

func (s *SecretsAPI) getBackendForUserSecretsWrite() (provider.SecretsBackend, error) {
	if s.activeBackendID == "" {
		if err := s.getBackendInfo(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	cfgInfo, err := s.backendConfigGetterForUserSecretsWrite(s.activeBackendID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[s.activeBackendID]
	if !ok {
		// This should never happen.
		return nil, errors.NotFoundf("secret backend %q", s.activeBackendID)
	}
	return s.backendGetter(&cfg)
}

// CreateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) CreateSecrets(_ struct{}) {}

// CreateSecrets creates new secrets.
func (s *SecretsAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	backend, err := s.getBackendForUserSecretsWrite()
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		ID, err := s.createSecret(backend, arg)
		result.Results[i].Result = ID
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
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

func (s *SecretsAPI) createSecret(backend provider.SecretsBackend, arg params.CreateSecretArg) (_ string, errOut error) {
	if arg.OwnerTag != "" && arg.OwnerTag != s.modelUUID {
		return "", errors.NotValidf("owner tag %q", arg.OwnerTag)
	}
	secretOwner := names.NewModelTag(s.modelUUID)
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
	v := coresecrets.NewSecretValue(arg.Content.Data)
	checksum, err := v.Checksum()
	if err != nil {
		return "", errors.Annotate(err, "calculating secret checksum")
	}
	arg.UpsertSecretArg.Content.Checksum = checksum
	revId, err := backend.SaveContent(context.TODO(), uri, 1, coresecrets.NewSecretValue(arg.Content.Data))
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return "", errors.Trace(err)
	}
	if err == nil {
		defer func() {
			if errOut != nil {
				// If we failed to create the secret, we should delete the
				// secret value from the backend.
				if err2 := backend.DeleteContent(context.TODO(), revId); err2 != nil &&
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

	md, err := s.secretsState.CreateSecret(uri, state.CreateSecretParams{
		Version:            secrets.Version,
		Owner:              secretOwner,
		UpdateSecretParams: fromUpsertParams(nil, arg.UpsertSecretArg),
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() {
		if errOut != nil {
			// If we failed to create the secret, we should delete the
			// secret metadata from the state.
			if _, err2 := s.secretsState.DeleteSecret(uri); err2 != nil {
				logger.Errorf("failed to cleanup secret %q: %v", uri, err2)
			}
		}
	}()
	err = s.secretsConsumer.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: successfulToken{},
		Scope:       secretOwner,
		Subject:     secretOwner,
		Role:        coresecrets.RoleManage,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return md.URI.String(), nil
}

func fromUpsertParams(autoPrune *bool, p params.UpsertSecretArg) state.UpdateSecretParams {
	var valueRef *coresecrets.ValueRef
	if p.Content.ValueRef != nil {
		valueRef = &coresecrets.ValueRef{
			BackendID:  p.Content.ValueRef.BackendID,
			RevisionID: p.Content.ValueRef.RevisionID,
		}
	}
	return state.UpdateSecretParams{
		AutoPrune:   autoPrune,
		LeaderToken: successfulToken{},
		Description: p.Description,
		Label:       p.Label,
		Params:      p.Params,
		Data:        p.Content.Data,
		ValueRef:    valueRef,
		Checksum:    p.Content.Checksum,
	}
}

// UpdateSecrets isn't on the v1 API.
func (s *SecretsAPIV1) UpdateSecrets(_ struct{}) {}

// UpdateSecrets creates new secrets.
func (s *SecretsAPI) UpdateSecrets(args params.UpdateUserSecretArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}
	backend, err := s.getBackendForUserSecretsWrite()
	if err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateSecret(backend, arg)
		if errors.Is(err, state.LabelExists) {
			err = errors.AlreadyExistsf("secret with name %q", *arg.Label)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsAPI) updateSecret(backend provider.SecretsBackend, arg params.UpdateUserSecretArg) (errOut error) {
	if err := arg.Validate(); err != nil {
		return errors.Trace(err)
	}
	var (
		uri *coresecrets.URI
		err error
	)
	if arg.URI != "" {
		uri, err = coresecrets.ParseURI(arg.URI)
	} else {
		uri, err = s.getSecretURI(s.modelUUID, arg.ExistingLabel)
	}
	if err != nil {
		return errors.Trace(err)
	}

	md, err := s.secretsState.GetSecret(uri)
	if err != nil {
		// Check if the uri exists or not.
		return errors.Trace(err)
	}
	if len(arg.Content.Data) > 0 {
		v := coresecrets.NewSecretValue(arg.Content.Data)
		checksum, err := v.Checksum()
		if err != nil {
			return errors.Annotate(err, "calculating secret checksum")
		}
		if checksum == md.LatestRevisionChecksum {
			logger.Debugf("no new revision for user secret with checksum %s", checksum)
			arg.Content.Data = nil
		} else {
			arg.Content.Checksum = checksum
			revId, err := backend.SaveContent(context.TODO(), uri, md.LatestRevision+1, coresecrets.NewSecretValue(arg.Content.Data))
			if err != nil && !errors.Is(err, errors.NotSupported) {
				return errors.Trace(err)
			}
			if err == nil {
				defer func() {
					if errOut != nil {
						// If we failed to update the secret, we should delete the
						// secret value from the backend for the new revision.
						if err2 := backend.DeleteContent(context.TODO(), revId); err2 != nil &&
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
	}

	// There might be nothing to update if the checksums matched above.
	if !arg.HasUpdate() {
		return nil
	}

	md, err = s.secretsState.UpdateSecret(uri, fromUpsertParams(arg.AutoPrune, arg.UpsertSecretArg))
	if err != nil {
		return errors.Trace(err)
	}
	if md.AutoPrune {
		// If the secret was updated, we need to delete the old unused secret revisions.
		revsToDelete, err := s.secretsState.ListUnusedSecretRevisions(uri)
		if err != nil {
			return errors.Trace(err)
		}
		pruneArg := params.DeleteSecretArg{URI: md.URI.String()}
		for _, rev := range revsToDelete {
			if rev == md.LatestRevision {
				// We don't want to delete the latest revision.
				continue
			}
			pruneArg.Revisions = append(pruneArg.Revisions, rev)
		}
		if len(pruneArg.Revisions) == 0 {
			return nil
		}
		pruneResult, err := s.RemoveSecrets(params.DeleteSecretArgs{Args: []params.DeleteSecretArg{pruneArg}})
		if err != nil {
			// We don't want to fail the update if we can't prune the unused secret revisions because they will be picked up later
			// when the secret has any new obsolute revisions.
			logger.Warningf("failed to prune unused secret revisions for %q: %v", uri, err)
		}
		if err = pruneResult.Combine(); err != nil {
			logger.Warningf("failed to prune unused secret revisions for %q: %v", uri, pruneResult.Combine())
		}
	}
	return nil
}

// RemoveSecrets isn't on the v1 API.
func (s *SecretsAPIV1) RemoveSecrets(_ struct{}) {}

// RemoveSecrets remove user secret.
func (s *SecretsAPI) RemoveSecrets(args params.DeleteSecretArgs) (params.ErrorResults, error) {
	// TODO(secrets): JUJU-4719.
	return commonsecrets.RemoveUserSecrets(
		s.secretsState, s.adminBackendConfigGetter,
		args, s.modelUUID,
		func(uri *coresecrets.URI) error {
			if err := s.checkCanWrite(); err != nil {
				return errors.Trace(err)
			}
			md, err := s.secretsState.GetSecret(uri)
			if err != nil {
				return errors.Trace(err)
			}
			// Can only delete model owned(user supplied) secrets.
			if md.OwnerTag != names.NewModelTag(s.modelUUID).String() {
				return apiservererrors.ErrPerm
			}
			return nil
		},
	)
}

// GrantSecret isn't on the v1 API.
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

func (s *SecretsAPI) getSecretURI(modelUUID, label string) (*coresecrets.URI, error) {
	results, err := s.secretsState.ListSecrets(state.SecretsFilter{
		Label:     &label,
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
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Applications)),
	}

	if arg.URI == "" && arg.Label == "" {
		return results, errors.New("must specify either URI or name")
	}

	if err := s.checkCanWrite(); err != nil {
		return results, errors.Trace(err)
	}

	var (
		uri *coresecrets.URI
		err error
	)
	if arg.URI != "" {
		uri, err = coresecrets.ParseURI(arg.URI)
	} else {
		uri, err = s.getSecretURI(s.modelUUID, arg.Label)
	}
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
