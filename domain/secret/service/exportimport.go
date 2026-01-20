// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
)

// GetSecretsForExport returns a result containing all the information needed to
// export secrets to a model description.
func (s *SecretService) GetSecretsForExport(ctx context.Context) (*SecretExport, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	secrets, secretRevisions, err := s.secretState.ListAllSecrets(ctx)
	if err != nil {
		return nil, errors.Errorf("loading secrets for export: %w", err)
	}

	remoteSecrets, err := s.secretState.AllRemoteSecrets(ctx)
	if err != nil {
		return nil, errors.Errorf("loading secrets for export: %w", err)
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
			Accessor: secret.SecretAccessor{
				Kind: secret.SecretAccessorKind(info.SubjectTypeID.String()),
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
			data, _, err := s.secretState.GetSecretValue(ctx, md.URI, rev.Revision)
			if err != nil {
				return nil, errors.Errorf("loading secret content for %q: %w", md.URI.ID, err)
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
		return nil, errors.Errorf("loading secret grants for export: %w", err)
	}
	for id, grants := range allGrants {
		secretAccess := make([]SecretAccess, len(grants))
		for i, grant := range grants {
			access := SecretAccess{
				Scope: secret.SecretAccessScope{
					Kind: secret.SecretAccessScopeKind(grant.ScopeTypeID.String()),
					ID:   grant.ScopeID,
				},
				Subject: secret.SecretAccessor{
					Kind: secret.SecretAccessorKind(grant.SubjectTypeID.String()),
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
		return nil, errors.Errorf("loading secret consumers for export: %w", err)
	}
	for id, consumers := range allConsumers {
		secretConsumers := make([]ConsumerInfo, len(consumers))
		for i, consumer := range consumers {
			info := ConsumerInfo{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					Label:           consumer.Label,
					CurrentRevision: consumer.CurrentRevision,
				},
				Accessor: secret.SecretAccessor{
					Kind: secret.SecretAccessorKind(consumer.SubjectTypeID.String()),
					ID:   consumer.SubjectID,
				},
			}
			secretConsumers[i] = info
		}
		allSecrets.Consumers[id] = secretConsumers
	}

	allRemoteConsumers, err := s.secretState.AllSecretRemoteConsumers(ctx)
	if err != nil {
		return nil, errors.Errorf("loading secret remote consumers for export: %w", err)
	}
	for id, consumers := range allRemoteConsumers {
		secretConsumers := make([]ConsumerInfo, len(consumers))
		for i, consumer := range consumers {
			info := ConsumerInfo{
				SecretConsumerMetadata: coresecrets.SecretConsumerMetadata{
					Label:           consumer.Label,
					CurrentRevision: consumer.CurrentRevision,
				},
				Accessor: secret.SecretAccessor{
					Kind: secret.SecretAccessorKind(consumer.SubjectTypeID.String()),
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.importRemoteSecrets(ctx, modelSecrets.RemoteSecrets); err != nil {
		return errors.Errorf("importing remote secrets: %w", err)
	}

	modelID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model uuid: %w", err)
	}

	for _, md := range modelSecrets.Secrets {
		revisions := modelSecrets.Revisions[md.URI.ID]
		content := modelSecrets.Content[md.URI.ID]
		if err := s.importSecretWithRevisions(ctx, modelID, md, revisions, content); err != nil {
			return errors.Errorf("saving secret %q: %w", md.URI.ID, err)
		}
		for _, sc := range modelSecrets.Consumers[md.URI.ID] {
			unitName, err := unit.NewName(sc.Accessor.ID)
			if err != nil {
				return errors.Errorf("invalid local secret consumer: %w", err)
			}
			if err := s.secretState.SaveSecretConsumer(ctx, md.URI, unitName, coresecrets.SecretConsumerMetadata{
				Label:           sc.Label,
				CurrentRevision: sc.CurrentRevision,
			}); err != nil {
				return errors.Errorf("saving secret consumer %q for %q: %w", sc.Accessor.ID, md.URI.ID, err)
			}
		}

		// TODO(secrets) - move to crossmodelrelation domain
		//for _, rc := range modelSecrets.RemoteConsumers[md.URI.ID] {
		//	unitName, err := unit.NewName(rc.Accessor.ID)
		//	if err != nil {
		//		return errors.Errorf("invalid remote secret consumer: %w", err)
		//	}
		//	if err := s.secretState.SaveSecretRemoteConsumer(ctx, md.URI, unitName, &coresecrets.SecretConsumerMetadata{
		//		Label:           rc.Label,
		//		CurrentRevision: rc.CurrentRevision,
		//	}); err != nil {
		//		return errors.Errorf("saving secret remote consumer %q for %q: %w", rc.Accessor.ID, md.URI.ID, err)
		//	}
		//}

		for _, access := range modelSecrets.Access[md.URI.ID] {
			p, err := s.grantParams(ctx, secret.SecretAccessParams{
				Scope: secret.SecretAccessScope{
					Kind: access.Scope.Kind,
					ID:   access.Scope.ID,
				},
				Subject: secret.SecretAccessor{
					Kind: access.Subject.Kind,
					ID:   access.Subject.ID,
				},
				Role: access.Role,
			})
			if err != nil {
				return errors.Capture(err)
			}
			if err := s.secretState.GrantAccess(ctx, md.URI, p); err != nil {
				return errors.Errorf("saving secret access for %s-%s for secret %q: %w",
					access.Subject.Kind, access.Subject.ID, md.URI.ID, err)

			}
		}
	}

	return nil
}

func (s *SecretService) importSecretWithRevisions(
	ctx context.Context, modelID coremodel.UUID, md *coresecrets.SecretMetadata,
	revisions []*coresecrets.SecretRevisionMetadata,
	content map[int]coresecrets.SecretData,
) (err error) {
	// Create secret metadata first.
	metaParams := secret.UpsertSecretParams{
		CreateTime:     md.CreateTime,
		UpdateTime:     md.UpdateTime,
		NextRotateTime: md.NextRotateTime,
		Description:    nilZeroPtr(md.Description),
		Label:          nilZeroPtr(md.Label),
		AutoPrune:      nilZeroPtr(md.AutoPrune),
		Checksum:       md.LatestRevisionChecksum,
		ExpireTime:     md.LatestExpireTime,
	}
	if md.RotatePolicy != "" && md.RotatePolicy != coresecrets.RotateNever {
		policy := secret.MarshallRotatePolicy(&md.RotatePolicy)
		metaParams.RotatePolicy = &policy
	}

	// Solve ownership.
	owner, err := s.getOwner(ctx, md.Owner.Kind, md.Owner.ID)
	if err != nil {
		return errors.Errorf("getting owner: %w", err)
	}

	// Create secret revisions and backend references.
	importRevisions := make([]secret.ImportRevision, len(revisions))
	var rollbackReferences []func() error
	defer func() {
		if err != nil {
			for _, rollBack := range rollbackReferences {
				if err := rollBack(); err != nil {
					s.logger.Warningf(ctx, "failed to roll back secret reference count: %v", err)
				}
			}
		}
	}()
	for i, rev := range revisions {
		revisionID, err := s.uuidGenerator()
		if err != nil {
			return errors.Capture(err)
		}
		params := secret.UpsertSecretParams{
			ValueRef:   rev.ValueRef,
			CreateTime: rev.CreateTime,
			UpdateTime: rev.CreateTime,
			RevisionID: ptr(revisionID.String()),
		}
		if i == len(revisions)-1 {
			params.Checksum = md.LatestRevisionChecksum
			params.ExpireTime = md.LatestExpireTime
		}

		if rev.ValueRef == nil {
			if data, ok := content[rev.Revision]; ok {
				params.Data = data
			} else {
				return errors.Errorf("missing content for secret %s/%d", md.URI.ID, rev.Revision)
			}
		}

		rollBack, err := s.secretBackendState.AddSecretBackendReference(ctx, params.ValueRef, modelID,
			revisionID.String())
		if err != nil {
			return errors.Capture(err)
		}
		rollbackReferences = append(rollbackReferences, rollBack)

		importRevisions[i] = secret.ImportRevision{
			Revision: rev.Revision,
			Params:   params,
		}
	}

	if err = s.secretState.ImportSecretWithRevision(ctx, md.Version, md.URI,
		owner,
		metaParams,
		importRevisions); err != nil {
		return errors.Errorf("saving secret %q: %w", md.URI.ID, err)
	}

	return nil
}

func (s *SecretService) importRemoteSecrets(ctx context.Context, remoteSecrets []RemoteSecret) error {
	return nil
}

func (s *SecretService) getOwner(ctx context.Context, kind coresecrets.OwnerKind, id string) (secret.Owner, error) {
	switch kind {
	case coresecrets.ModelOwner:
		return secret.Owner{
			Kind: kind,
			UUID: id,
		}, nil
	case coresecrets.ApplicationOwner:
		uuid, err := s.getApplicationUUIDByName(ctx, id)
		if err != nil {
			return secret.Owner{}, errors.Errorf("getting application uuid for %q: %w", id, err)
		}
		return secret.Owner{
			Kind: kind,
			UUID: uuid,
		}, nil
	case coresecrets.UnitOwner:
		uuid, err := s.getUnitUUIDByName(ctx, id)
		if err != nil {
			return secret.Owner{}, errors.Errorf("getting unit uuid for %q: %w", id, err)
		}
		return secret.Owner{
			Kind: kind,
			UUID: uuid,
		}, nil
	}

	return secret.Owner{}, errors.Errorf("unknown owner kind %q for id %q", kind, id)
}

// nilZeroPtr returns a pointer to the value if it is not the zero value,
// otherwise returns nil.
func nilZeroPtr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
