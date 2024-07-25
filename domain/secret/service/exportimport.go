// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
)

// GetSecretsForExport returns a result containing all the information needed to
// export secrets to a model description.
func (s *SecretService) GetSecretsForExport(ctx context.Context) (*SecretExport, error) {
	secrets, secretRevisions, err := s.st.ListSecrets(ctx, nil, nil, secret.NilLabels)
	if err != nil {
		return nil, errors.Annotate(err, "loading secrets for export")
	}

	remoteSecrets, err := s.st.AllRemoteSecrets(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "loading secrets for export")
	}

	allSecrets := &SecretExport{
		Secrets:         secrets,
		Revisions:       make(map[string][]*coresecrets.SecretRevisionMetadata),
		Content:         make(map[string]map[int]coresecrets.SecretData),
		Access:          make(map[string][]SecretAccess),
		Consumers:       make(map[string][]ConsumerInfo),
		RemoteConsumers: make(map[string][]ConsumerInfo),
		RemoteSecrets:   make([]RemoteSecret, len(remoteSecrets)),
	}

	for i, info := range remoteSecrets {
		allSecrets.RemoteSecrets[i] = RemoteSecret{
			URI:             info.URI,
			Label:           info.Label,
			CurrentRevision: info.CurrentRevision,
			LatestRevision:  info.LatestRevision,
			Accessor: SecretAccessor{
				Kind: SecretAccessorKind(info.SubjectTypeID.String()),
				ID:   info.SubjectID,
			},
		}
	}

	for i, md := range secrets {
		revs := secretRevisions[i]
		allSecrets.Revisions[md.URI.ID] = revs
		for _, rev := range revs {
			if rev.ValueRef != nil {
				continue
			}
			data, _, err := s.st.GetSecretValue(ctx, md.URI, rev.Revision)
			if err != nil {
				return nil, errors.Annotatef(err, "loading secret content for %q", md.URI.ID)
			}
			if len(data) == 0 {
				// Should not happen.
				return nil, errors.Errorf("unexpected empty secret content for %q", md.URI.ID)
			}
			allSecrets.Content[md.URI.ID] = map[int]coresecrets.SecretData{
				rev.Revision: data,
			}
		}
	}

	allGrants, err := s.st.AllSecretGrants(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "loading secret grants for export")
	}
	for id, grants := range allGrants {
		secretAccess := make([]SecretAccess, len(grants))
		for i, grant := range grants {
			access := SecretAccess{
				Scope: SecretAccessScope{
					Kind: SecretAccessScopeKind(grant.ScopeTypeID.String()),
					ID:   grant.ScopeID,
				},
				Subject: SecretAccessor{
					Kind: SecretAccessorKind(grant.SubjectTypeID.String()),
					ID:   grant.SubjectID,
				},
				Role: coresecrets.SecretRole(grant.RoleID.String()),
			}
			secretAccess[i] = access
		}
		allSecrets.Access[id] = secretAccess
	}

	allConsumers, err := s.st.AllSecretConsumers(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "loading secret consumers for export")
	}
	for id, consumers := range allConsumers {
		secretConsumers := make([]ConsumerInfo, len(consumers))
		for i, consumer := range consumers {
			info := ConsumerInfo{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					Label:           consumer.Label,
					CurrentRevision: consumer.CurrentRevision,
				},
				Accessor: SecretAccessor{
					Kind: SecretAccessorKind(consumer.SubjectTypeID.String()),
					ID:   consumer.SubjectID,
				},
			}
			secretConsumers[i] = info
		}
		allSecrets.Consumers[id] = secretConsumers
	}

	allRemoteConsumers, err := s.st.AllSecretRemoteConsumers(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "loading secret remote consumers for export")
	}
	for id, consumers := range allRemoteConsumers {
		secretConsumers := make([]ConsumerInfo, len(consumers))
		for i, consumer := range consumers {
			info := ConsumerInfo{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					Label:           consumer.Label,
					CurrentRevision: consumer.CurrentRevision,
				},
				Accessor: SecretAccessor{
					Kind: SecretAccessorKind(consumer.SubjectTypeID.String()),
					ID:   consumer.SubjectID,
				},
			}
			secretConsumers[i] = info
		}
		allSecrets.RemoteConsumers[id] = secretConsumers
	}

	return allSecrets, nil
}

// ImportSecrets saves the supplied secret details to the model.
func (s *SecretService) ImportSecrets(context.Context, *SecretExport) error {
	// TODO(secrets)
	return nil
}
