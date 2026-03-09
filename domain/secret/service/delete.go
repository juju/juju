// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// DeleteObsoleteUserSecretRevisions schedules pruning of obsolete user secret
// revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	jobUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}

	return s.secretState.ScheduleObsoleteUserSecretRevisionsPruning(
		ctx, jobUUID.String(), s.clock.Now().UTC(),
	)
}

// DeleteSecret schedules removal of the specified secret or specific revisions.
// If revisions is nil or empty, the entire secret will be removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params secret.DeleteSecretParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		// validate if provided revisions exist before scheduling job
		if len(params.Revisions) > 0 {
			for _, revision := range params.Revisions {
				if _, err := s.secretState.GetSecretRevisionID(ctx, uri, revision); err != nil {
					return errors.Capture(err)
				}
			}
		}

		jobID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}

		err = s.secretState.ScheduleUserSecretRemoval(ctx, jobID.String(), uri, params.Revisions, s.clock.Now().UTC())
		if err != nil {
			return errors.Errorf("scheduling job for removal of user secret %q: %w", uri.String(), err)
		}

		s.logger.Infof(ctx, "scheduled removal of user secret %q", uri.String())
		return nil
	})
}
