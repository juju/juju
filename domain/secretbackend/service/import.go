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

	backendIDs := make(map[string]string)
	for _, ref := range refs {
		backendID, ok := backendIDs[ref.BackendName]
		if !ok {
			backend, err := s.st.GetSecretBackend(ctx, secretbackend.BackendIdentifier{Name: ref.BackendName})
			if err != nil {
				return errors.Errorf("looking up secret backend %q: %w", ref.BackendName, err)
			}
			backendID = backend.ID
			backendIDs[ref.BackendName] = backendID
		}

		if _, err := s.st.AddSecretBackendReference(
			ctx, &secrets.ValueRef{BackendID: backendID}, modelUUID, ref.SecretRevisionUUID, ref.SecretID,
		); err != nil {
			return errors.Errorf("adding secret backend reference for revision %q: %w", ref.SecretRevisionUUID, err)
		}
	}
	return nil
}
