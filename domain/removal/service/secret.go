// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
)

// SecretModelState describes functionality required for deleting
// secret data asscociated with removed entities.
type SecretModelState interface {
	// DeleteApplicationOwnedSecretContent deletes content for all
	// secrets owned by the application with the input UUID.
	// It must only be called in the context of application removal.
	DeleteApplicationOwnedSecretContent(ctx context.Context, aUUID string) error

	// DeleteUnitOwnedSecretContent deletes content for all
	// secrets owned by the unit with the input UUID.
	// It must only be called in the context of unit removal.
	DeleteUnitOwnedSecretContent(ctx context.Context, uUUID string) error

	// DeleteApplicationOwnedSecrets deletes all data for secrets owned by the
	// application with the input UUID.
	// This does not include secret content, which should be handled by
	// interaction with the secret back-end.
	DeleteApplicationOwnedSecrets(ctx context.Context, aUUID string) error

	// DeleteUnitOwnedSecrets deletes all data for secrets owned by the
	// unit with the input UUID.
	// This does not include secret content, which should be handled by
	// interaction with the secret back-end.
	DeleteUnitOwnedSecrets(ctx context.Context, uUUID string) error

	// GetApplicationOwnedSecretRevisionRefs returns the back-end value
	// references for secret revisions owned by the application with
	// the input UUID.
	GetApplicationOwnedSecretRevisionRefs(ctx context.Context, aUUID string) ([]string, error)

	// GetUnitOwnedSecretRevisionRefs returns the back-end value references
	// for secret revisions owned by the application with the input UUID.
	GetUnitOwnedSecretRevisionRefs(ctx context.Context, uUUID string) ([]string, error)

	// DeleteUserSecretRevisions deletes the specified revisions of the user
	// secret with the input URI. If revisions is nil or empty, all revisions
	// are deleted. Returns the revision UUIDs that were deleted.
	DeleteUserSecretRevisions(ctx context.Context, uri *coresecrets.URI, revisions []int) ([]string, error)

	// DeleteObsoleteUserSecretRevisions deletes all obsolete revisions of
	// auto-prune user secrets. Returns the deleted revision UUIDs for
	// back-end content cleanup.
	DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error)

	// GetUserSecretRevisionRefs returns the back-end value
	// references for the specified revision UUIDs.
	GetUserSecretRevisionRefs(ctx context.Context, revisionUUIDs []string) ([]string, error)

	// DeleteUserSecretRevisionRef removes the back-end value reference
	// for the specified deleted revision UUID.
	DeleteUserSecretRevisionRef(ctx context.Context, revisionUUID string) error
}

// processUserSecretRemovalJob deletes a user secret or specific revisions.
// The EntityUUID in the job contains the full secret URI string.
// Optional revisions can be specified in job.Arg["revisions"] as []int.
func (s *Service) processUserSecretRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.UserSecretJob {
		return errors.Errorf("job type: %q not valid for user secret removal", job.RemovalType).
			Add(removalerrors.RemovalJobTypeNotValid)
	}

	uri, err := coresecrets.ParseURI(job.EntityUUID)
	if err != nil {
		return errors.Errorf("parsing secret URI from job: %w", err)
	}

	// Extract revisions from job args if provided
	var revisions []int
	if job.Arg != nil {
		if revs, ok := job.Arg["revisions"]; ok && revs != nil {
			// Handle different numeric types that might come from JSON
			switch v := revs.(type) {
			case []int:
				revisions = v
			case []any:
				revisions = make([]int, 0, len(v))
				for _, r := range v {
					switch rv := r.(type) {
					case int:
						revisions = append(revisions, rv)
					case float64:
						revisions = append(revisions, int(rv))
					case int64:
						revisions = append(revisions, int(rv))
					}
				}
			}
		}
	}

	if err := s.deleteUserSecretRevisions(ctx, uri, revisions); err != nil {
		return errors.Errorf("deleting user secret %q revisions %v: %w", job.EntityUUID, revisions, err)
	}

	return nil
}

// processObsoleteUserSecretRevisionsJob prunes all obsolete revisions of
// auto-prune user secrets and cleans up back-end content as needed.
func (s *Service) processObsoleteUserSecretRevisionsJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ObsoleteUserSecretRevisionsJob {
		return errors.Errorf("job type: %q not valid for obsolete secret revisions pruning", job.RemovalType).
			Add(removalerrors.RemovalJobTypeNotValid)
	}

	deletedRevisionUUIDs, err := s.modelState.DeleteObsoleteUserSecretRevisions(ctx)
	if err != nil {
		return errors.Errorf("deleting obsolete user secret revisions: %w", err)
	}

	return s.deleteSecretRevisionContent(ctx, deletedRevisionUUIDs)
}

func (s *Service) deleteApplicationOwnedSecrets(ctx context.Context, aUUID coreapplication.UUID) error {
	sb, err := s.getSecretBackend(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if sb == nil {
		if err := s.modelState.DeleteApplicationOwnedSecretContent(ctx, aUUID.String()); err != nil {
			return errors.Errorf("deleting secret content: %w", err)
		}
	} else {
		ids, err := s.modelState.GetApplicationOwnedSecretRevisionRefs(ctx, aUUID.String())
		if err != nil {
			return errors.Errorf("getting secret revision back-end refs: %w", err)
		}

		// For external content, make a best-effort - just log any errors.
		for _, id := range ids {
			if err := sb.DeleteContent(ctx, id); err != nil {
				s.logger.Warningf(ctx, "failed to delete secret content for external reference %q: %v", id, err)
			}
		}
	}

	if err := s.modelState.DeleteApplicationOwnedSecrets(ctx, aUUID.String()); err != nil {
		return errors.Errorf("deleting secret metadata: %w", err)
	}

	return nil
}

func (s *Service) deleteUnitOwnedSecrets(ctx context.Context, uUUID coreunit.UUID) error {
	sb, err := s.getSecretBackend(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if sb == nil {
		if err := s.modelState.DeleteUnitOwnedSecretContent(ctx, uUUID.String()); err != nil {
			return errors.Errorf("deleting secret content: %w", err)
		}
	} else {
		ids, err := s.modelState.GetUnitOwnedSecretRevisionRefs(ctx, uUUID.String())
		if err != nil {
			return errors.Errorf("getting secret revision back-end refs: %w", err)
		}

		// For external content, make a best-effort - just log any errors.
		for _, id := range ids {
			if err := sb.DeleteContent(ctx, id); err != nil {
				s.logger.Warningf(ctx, "failed to delete secret content for external reference %q: %v", id, err)
			}
		}
	}

	if err := s.modelState.DeleteUnitOwnedSecrets(ctx, uUUID.String()); err != nil {
		return errors.Errorf("deleting secret metadata: %w", err)
	}

	return nil
}

func (s *Service) getSecretBackend(ctx context.Context) (provider.SecretsBackend, error) {
	_, modelBackendCfg, err := s.controllerState.GetActiveModelSecretBackend(ctx, s.modelUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting model secret backend: %w", err)
	}

	// See comment in domain/removal/state/model/secret.go.
	// This trapdoor should not exist, and a proper DB-backed
	// implementation of the Juju secret back-end should replace it.
	if modelBackendCfg.BackendType == juju.BackendType {
		return nil, nil
	}

	p, err := s.secretBackendProviderGetter(modelBackendCfg.BackendType)
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = p.Initialise(modelBackendCfg)
	if err != nil {
		return nil, errors.Errorf("initialising secrets provider: %w", err)
	}

	sb, err := p.NewBackend(modelBackendCfg)
	return sb, errors.Capture(err)
}

func (s *Service) deleteUserSecretRevisions(ctx context.Context, uri *coresecrets.URI, revisions []int) error {
	// Delete the specified revisions (or all if revisions is nil)
	deletedRevisionUUIDs, err := s.modelState.DeleteUserSecretRevisions(ctx, uri, revisions)
	if err != nil {
		return errors.Errorf("deleting secret revisions: %w", err)
	}
	s.logger.Infof(ctx, "deleted secret %s revisions %v", uri.String(), deletedRevisionUUIDs)

	return s.deleteSecretRevisionContent(ctx, deletedRevisionUUIDs)
}

// deleteSecretRevisionContent deletes the secret content from the backend.
func (s *Service) deleteSecretRevisionContent(ctx context.Context, deletedRevisionUUIDs []string) error {
	// If using external backend, clean up the content
	sb, err := s.getSecretBackend(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if sb == nil {
		// No backend configured, secret content is already deleted with the revision
		return nil
	}

	// Get the backend references for the deleted revisions
	refs, err := s.modelState.GetUserSecretRevisionRefs(ctx, deletedRevisionUUIDs)
	if err != nil {
		return errors.Errorf("getting secret revision back-end refs: %w", err)
	}

	// For external content, make a best-effort - just log any errors.
	for _, ref := range refs {
		if err := sb.DeleteContent(ctx, ref); err != nil {
			s.logger.Warningf(ctx, "failed to delete secret content for external reference %q: %v", ref, err)
			continue
		}

		if err := s.modelState.DeleteUserSecretRevisionRef(ctx, ref); err != nil {
			s.logger.Warningf(ctx, "failed to delete secret backend reference for revision %q: %v", ref, err)
		}
	}
	return nil
}
