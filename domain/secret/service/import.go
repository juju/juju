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

// ImportSecrets saves the supplied secret details to the model.
func (s *SecretService) ImportSecrets(ctx context.Context, modelSecrets *SecretImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

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
	importRevisions := make([]secret.UpsertRevisionParams, len(revisions))
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
			UpdateTime: rev.UpdateTime,
			RevisionID: new(revisionID.String()),
			ExpireTime: rev.ExpireTime,
		}
		if i == len(revisions)-1 {
			params.Checksum = md.LatestRevisionChecksum
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

		importRevisions[i] = secret.UpsertRevisionParams{
			Revision:   rev.Revision,
			RevisionID: params.RevisionID,
			CreateTime: params.CreateTime,
			UpdateTime: params.UpdateTime,
			ExpireTime: params.ExpireTime,
			ValueRef:   params.ValueRef,
			Data:       params.Data,
			Checksum:   params.Checksum,
		}
	}

	if err = s.secretState.ImportSecretWithRevisions(ctx, md.Version, md.URI,
		owner,
		metaParams,
		importRevisions); err != nil {
		return errors.Errorf("saving secret %q: %w", md.URI.ID, err)
	}

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
			UUID: uuid.String(),
		}, nil
	case coresecrets.UnitOwner:
		uuid, err := s.getUnitUUIDByName(ctx, id)
		if err != nil {
			return secret.Owner{}, errors.Errorf("getting unit uuid for %q: %w", id, err)
		}
		return secret.Owner{
			Kind: kind,
			UUID: uuid.String(),
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
