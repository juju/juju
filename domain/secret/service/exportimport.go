// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secret"
)

// GetSecretsForExport returns a result containing all the information needed to
// export secrets to a model description.
func (s *SecretService) GetSecretsForExport(ctx context.Context) (*SecretExport, error) {
	secrets, secretRevisions, err := s.secretState.ListSecrets(ctx, nil, nil, secret.NilLabels)
	if err != nil {
		return nil, errors.Annotate(err, "loading secrets for export")
	}

	remoteSecrets, err := s.secretState.AllRemoteSecrets(ctx)
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
			var data coresecrets.SecretData
			err := s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
				var err error
				data, _, err = s.secretState.GetSecretValue(ctx, md.URI, rev.Revision)
				return err
			})
			if err != nil {
				return nil, errors.Annotatef(err, "loading secret content for %s/%d", md.URI.ID, rev.Revision)
			}
			if len(data) == 0 {
				// Should not happen.
				return nil, errors.Errorf("unexpected empty secret content for %q", md.URI.ID)
			}
			if _, ok := allSecrets.Content[md.URI.ID]; !ok {
				allSecrets.Content[md.URI.ID] = make(map[int]coresecrets.SecretData)
			}
			allSecrets.Content[md.URI.ID][rev.Revision] = data
		}
	}

	allGrants, err := s.secretState.AllSecretGrants(ctx)
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

	allConsumers, err := s.secretState.AllSecretConsumers(ctx)
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

	allRemoteConsumers, err := s.secretState.AllSecretRemoteConsumers(ctx)
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
func (s *SecretService) ImportSecrets(ctx context.Context, modelSecrets *SecretExport) error {
	if err := s.importRemoteSecrets(ctx, modelSecrets.RemoteSecrets); err != nil {
		return errors.Annotate(err, "importing remote secrets")
	}

	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Annotate(err, "getting model uuid")
	}
	modelID := coremodel.UUID(modelUUID)

	for _, md := range modelSecrets.Secrets {
		revisions := modelSecrets.Revisions[md.URI.ID]
		content := modelSecrets.Content[md.URI.ID]
		if err := s.importSecretRevisions(ctx, modelID, md, revisions, content); err != nil {
			return errors.Annotatef(err, "saving secret %q", md.URI.ID)
		}
		for _, sc := range modelSecrets.Consumers[md.URI.ID] {
			err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
				return s.secretState.SaveSecretConsumer(ctx, md.URI, sc.Accessor.ID, &coresecrets.SecretConsumerMetadata{
					Label:           sc.Label,
					CurrentRevision: sc.CurrentRevision,
				})
			})
			if err != nil {
				return errors.Annotatef(err, "saving secret consumer %q for %q", sc.Accessor.ID, md.URI.ID)
			}
		}

		for _, rc := range modelSecrets.RemoteConsumers[md.URI.ID] {
			err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
				return s.secretState.SaveSecretRemoteConsumer(ctx, md.URI, rc.Accessor.ID, &coresecrets.SecretConsumerMetadata{
					Label:           rc.Label,
					CurrentRevision: rc.CurrentRevision,
				})
			})
			if err != nil {
				return errors.Annotatef(err, "saving secret remote consumer %q for %q", rc.Accessor.ID, md.URI.ID)
			}
		}

		for _, access := range modelSecrets.Access[md.URI.ID] {
			p := grantParams(SecretAccessParams{
				Scope: SecretAccessScope{
					Kind: access.Scope.Kind,
					ID:   access.Scope.ID,
				},
				Subject: SecretAccessor{
					Kind: access.Subject.Kind,
					ID:   access.Subject.ID,
				},
				Role: access.Role,
			})
			err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
				return s.secretState.GrantAccess(ctx, md.URI, p)
			})
			if err != nil {
				return errors.Annotatef(err, "saving secret access for %s-%s for secret %q",
					access.Subject.Kind, access.Subject.ID, md.URI.ID)
			}
		}
	}

	return nil
}

func (s *SecretService) importSecretRevisions(
	ctx context.Context, modelID coremodel.UUID, md *coresecrets.SecretMetadata,
	revisions []*coresecrets.SecretRevisionMetadata,
	content map[int]coresecrets.SecretData,
) error {
	for i, rev := range revisions {
		params := secret.UpsertSecretParams{
			ValueRef: rev.ValueRef,
		}
		if i == len(revisions)-1 {
			params.Checksum = md.LatestRevisionChecksum
		}
		if rev.ValueRef == nil {
			if data, ok := content[rev.Revision]; ok {
				params.Data = data
			} else {
				// Should never happen.
				return errors.Errorf("missing content for secret %s/%d", md.URI.ID, rev.Revision)
			}
		}
		// The expiry time of the most recent revision
		// is included in the export.
		if i == len(revisions)-1 {
			params.ExpireTime = md.LatestExpireTime
		}
		if i == 0 {
			if err := s.createImportedSecret(ctx, modelID, md, params); err != nil {
				return errors.Annotatef(err, "cannot import secret %q", md.URI.ID)
			}
			continue
		}
		revisionID, err := s.uuidGenerator()
		if err != nil {
			return errors.Trace(err)
		}
		params.RevisionID = ptr(revisionID.String())

		rollBack := func() error { return nil }
		if params.ValueRef != nil || len(params.Data) != 0 {
			rollBack, err = s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, params.ValueRef, modelID, revisionID.String())
			if err != nil {
				return errors.Trace(err)
			}
		}
		err = s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
			return s.secretState.UpdateSecret(ctx, md.URI, params)
		})
		if err != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf("failed to roll back secret reference count: %v", err)
			}
			return errors.Annotatef(err, "cannot import secret %q revision %d", md.URI.ID, rev.Revision)
		}
	}
	return nil
}

func (s *SecretService) createImportedSecret(
	ctx context.Context, modelID coremodel.UUID, md *coresecrets.SecretMetadata, params secret.UpsertSecretParams,
) (errOut error) {
	params.NextRotateTime = md.NextRotateTime
	if md.RotatePolicy != "" && md.RotatePolicy != coresecrets.RotateNever {
		policy := secret.MarshallRotatePolicy(&md.RotatePolicy)
		params.RotatePolicy = &policy
	}
	if md.Description != "" {
		params.Description = &md.Description
	}
	if md.Label != "" {
		params.Label = &md.Label
	}
	if md.AutoPrune {
		params.AutoPrune = &md.AutoPrune
	}

	revisionID, err := s.uuidGenerator()
	if err != nil {
		return errors.Trace(err)
	}
	params.RevisionID = ptr(revisionID.String())

	rollBack, err := s.secretBackendReferenceMutator.AddSecretBackendReference(ctx, params.ValueRef, modelID, revisionID.String())
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		if errOut != nil {
			if err := rollBack(); err != nil {
				s.logger.Warningf("failed to roll back secret reference count: %v", err)
			}
		}
	}()
	switch md.Owner.Kind {
	case coresecrets.ModelOwner:
		err = s.secretState.CreateUserSecret(ctx, md.Version, md.URI, params)
	case coresecrets.ApplicationOwner:
		err = s.secretState.CreateCharmApplicationSecret(ctx, md.Version, md.URI, md.Owner.ID, params)
	case coresecrets.UnitOwner:
		err = s.secretState.CreateCharmUnitSecret(ctx, md.Version, md.URI, md.Owner.ID, params)
	default:
		// Should never happen.
		return errors.Errorf("cannot import secret %q with owner kind %q", md.URI.ID, md.Owner.Kind)
	}
	return err
}

func (s *SecretService) importRemoteSecrets(ctx context.Context, remoteSecrets []RemoteSecret) error {
	remoteSecretLatest := make(map[*coresecrets.URI]int)
	for _, rs := range remoteSecrets {
		remoteSecretLatest[rs.URI] = rs.LatestRevision
	}
	for uri, latest := range remoteSecretLatest {
		if err := s.secretState.UpdateRemoteSecretRevision(ctx, uri, latest); err != nil {
			return errors.Annotatef(err, "saving remote secret reference for %q", uri)
		}
	}
	for _, rs := range remoteSecrets {
		err := s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
			return s.secretState.SaveSecretConsumer(ctx, rs.URI, rs.Accessor.ID, &coresecrets.SecretConsumerMetadata{
				Label:           rs.Label,
				CurrentRevision: rs.CurrentRevision,
			})
		})
		if err != nil {
			return errors.Annotatef(err, "saving remote consumer %s-%s for secret %q",
				rs.Accessor.Kind, rs.Accessor.ID, rs.URI.ID)
		}
	}
	return nil
}
