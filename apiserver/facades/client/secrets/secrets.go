// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      string

	state         SecretsState
	mu            sync.Mutex
	backends      map[string]provider.SecretsBackend
	backendGetter func(string) (provider.SecretsBackend, error)
}

func (s *SecretsAPI) checkCanRead() error {
	canRead, err := s.authorizer.HasPermission(permission.ReadAccess, names.NewModelTag(s.modelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (s *SecretsAPI) checkCanAdmin() error {
	canAdmin, err := common.HasModelAdmin(s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if !canAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
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
	metadata, err := s.state.ListSecrets(filter)
	if err != nil {
		return params.ListSecretResults{}, errors.Trace(err)
	}
	revisionMetadata := make(map[string][]*coresecrets.SecretRevisionMetadata)
	for _, md := range metadata {
		if arg.Filter.Revision == nil {
			revs, err := s.state.ListSecretRevisions(md.URI)
			if err != nil {
				return params.ListSecretResults{}, errors.Trace(err)
			}
			revisionMetadata[md.URI.ID] = revs
			continue
		}
		rev, err := s.state.GetSecretRevision(md.URI, *arg.Filter.Revision)
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
			secretResult.Revisions = append(secretResult.Revisions, params.SecretRevision{
				Revision:   r.Revision,
				CreateTime: r.CreateTime,
				UpdateTime: r.UpdateTime,
				ExpireTime: r.ExpireTime,
			})
		}
		if arg.ShowSecrets {
			rev := m.LatestRevision
			if arg.Filter.Revision != nil {
				rev = *arg.Filter.Revision
			}
			val, ref, err := s.state.GetSecretValue(m.URI, rev)
			if ref != nil {
				val, err = s.secretContentFromBackend(ref)
			}
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

func (s *SecretsAPI) getBackend(ID string) (provider.SecretsBackend, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.backends[ID]
	if ok {
		return b, nil
	}
	b, err := s.backendGetter(ID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	s.backends[ID] = b
	return b, nil
}

func (s *SecretsAPI) secretContentFromBackend(ref *coresecrets.ValueRef) (coresecrets.SecretValue, error) {
	backend, err := s.getBackend(ref.BackendID)
	if err != nil {
		return nil, err
	}
	return backend.GetContent(context.TODO(), ref.RevisionID)
}
