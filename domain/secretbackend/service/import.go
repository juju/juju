// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/errors"
)

// ImportSecretBackendReferences records the model's per-revision secret
// backend references on the target controller. The model's own secret backend
// is set during the model bootstrap step of the v8 import; this only links
// existing secret revisions to their backend by the target's backend ID.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s *Service) ImportSecretBackendReferences(
	ctx context.Context, modelUUID coremodel.UUID, refs []coremodelmigration.SecretBackendReference,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(refs) == 0 {
		return nil
	}

	revisionMap, err := s.resolveBackendUUIDsByRevision(ctx, refs)
	if err != nil {
		return errors.Capture(err)
	}

	for _, ref := range refs {
		if _, err := s.st.AddSecretBackendReference(
			ctx, &secrets.ValueRef{BackendID: revisionMap[ref.SecretRevisionUUID]}, modelUUID, ref.SecretRevisionUUID, ref.SecretID,
		); err != nil {
			return errors.Errorf("adding secret backend reference for revision %q: %w", ref.SecretRevisionUUID, err)
		}
	}
	return nil
}

// GetSecretBackendReferenceMapping resolves the target controller's backend
// UUID for each secret revision carried in refs, keyed by secret revision UUID.
//
// It is read-only: it looks up each distinct backend by name and writes no
// state. The v8 migration import driver in internal/migration calls it, after
// the controller-scoped data is applied and before the model-DB insert, to
// rewrite the model-DB payload's secret_value_ref and secret_deleted_value_ref
// BackendUUID fields from the source controller's backend UUIDs to the
// target's. Returns a nil map for an empty refs slice.
func (s *Service) GetSecretBackendReferenceMapping(
	ctx context.Context, refs []coremodelmigration.SecretBackendReference,
) (map[string]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.resolveBackendUUIDsByRevision(ctx, refs)
}

// resolveBackendUUIDsByRevision maps each ref's secret revision UUID to its
// target backend UUID, looking up each distinct backend name once. It performs
// no writes. Returns a nil map when refs is empty.
func (s *Service) resolveBackendUUIDsByRevision(
	ctx context.Context, refs []coremodelmigration.SecretBackendReference,
) (map[string]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	backendIDs := make(map[string]string)
	revisionMap := make(map[string]string, len(refs))
	for _, ref := range refs {
		backendID, ok := backendIDs[ref.BackendName]
		if !ok {
			backend, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{Name: ref.BackendName})
			if err != nil {
				return nil, errors.Errorf("looking up secret backend %q: %w", ref.BackendName, err)
			}
			backendID = backend.ID
			backendIDs[ref.BackendName] = backendID
		}
		revisionMap[ref.SecretRevisionUUID] = backendID
	}
	return revisionMap, nil
}
