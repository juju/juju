// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	deletedRevisionIDs, err := s.secretState.DeleteObsoleteUserSecretRevisions(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err = s.secretBackendState.RemoveSecretBackendReference(ctx, deletedRevisionIDs...); err != nil {
		// We don't want to error out if we can't remove the backend reference.
		s.logger.Errorf(ctx, "failed to remove secret backend reference for deleted obsolete user secret revisions: %v", err)
	}
	return nil
}

// DeleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params DeleteSecretParams) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		// TODO (manadart 2024-11-29): This context naming is nasty,
		// but will be removed with RunAtomic.
		if err := s.secretState.RunAtomic(innerCtx, func(innerInnerCtx domain.AtomicContext) error {
			return s.secretState.DeleteSecret(innerInnerCtx, uri, params.Revisions)
		}); err != nil {
			return errors.Errorf("deleting secret %q: %w", uri.ID, err)
		}
		return nil
	})
}
