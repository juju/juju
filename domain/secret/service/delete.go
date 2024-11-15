// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
)

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	deletedRevisionIDs, err := s.secretState.DeleteObsoleteUserSecretRevisions(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err = s.secretBackendReferenceMutator.RemoveSecretBackendReference(ctx, deletedRevisionIDs...); err != nil {
		// We don't want to error out if we can't remove the backend reference.
		s.logger.Errorf(context.TODO(), "failed to remove secret backend reference for deleted obsolete user secret revisions: %v", err)
	}
	return nil
}

// DeleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params DeleteSecretParams) error {
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}
	if err := s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.secretState.DeleteSecret(ctx, uri, params.Revisions)
	}); err != nil {
		return errors.Annotatef(err, "deleting secret %q", uri.ID)
	}
	return nil
}
