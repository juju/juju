// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	errors2 "github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
)

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	s.logger.Tracef(ctx, "deleting obsolete user secret revisions")

	// Get list of obsolete revisions with their metadata.
	obsoleteRevs, err := s.getObsoleteRevisionsForDeletion(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if len(obsoleteRevs) == 0 {
		s.logger.Tracef(ctx, "no obsolete revisions found for deletion")
		return nil
	}

	s.logger.Infof(ctx, "%d obsolete revisions found for deletion", len(obsoleteRevs))

	// Delete from backend
	providersToCleanUp := make(map[string]provider.SecretRevisions)
	if err = s.deleteObsoleteRevisionsFromBackend(ctx, obsoleteRevs, providersToCleanUp); err != nil {
		return errors.Capture(err)
	}

	// TODO Delete backend references for deleted revisions
	if len(providersToCleanUp) > 0 {
		s.logger.Warningf(ctx, "provider cleanup needed for %d backend(s), but not yet implemented", len(providersToCleanUp))
	}

	// Delete from database
	deletedRevisionIDs, err := s.secretState.DeleteObsoleteUserSecretRevisions(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Remove secret backend reference
	if err = s.secretBackendState.RemoveSecretBackendReference(ctx, deletedRevisionIDs...); err != nil {
		// We don't want to error out if we can't remove the backend reference.
		s.logger.Errorf(ctx, "failed to remove secret backend reference for deleted obsolete user secret revisions: %v", err)
	}

	s.logger.Infof(ctx, "deleted %d obsolete user secret revision(s)", len(deletedRevisionIDs))
	return nil
}

// DeleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params secret.DeleteSecretParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	backend, backendID, err := s.getBackendForUserSecrets(ctx, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	revs, err := s.findRevisionsToDelete(ctx, uri, params)
	if err != nil {
		return errors.Capture(err)
	}

	s.mapRevisions(revs)
	err = s.deleteRevisionsFromBackend(ctx, uri, revs, backend, backendID)
	if err != nil {
		return errors.Capture(err)
	}

	// TODO Implement cleanup secrets from service account role

	withCaveat, err := s.getManagementCaveat(ctx, uri, params.Accessor)
	if err != nil {
		return errors.Capture(err)
	}

	return withCaveat(ctx, func(innerCtx context.Context) error {
		if err := s.secretState.DeleteSecret(innerCtx, uri, params.Revisions); err != nil {
			return errors.Errorf("deleting secret %q: %w", uri.ID, err)
		}
		s.logger.Infof(innerCtx, "deleted secret %q from database", uri.ID)
		return nil
	})

	// TODO Delete backend references for deleted revisions
}

func (s *SecretService) findRevisionsToDelete(
	ctx context.Context,
	uri *secrets.URI,
	params secret.DeleteSecretParams,
) ([]*secrets.SecretRevisionMetadata, error) {
	var revs []*secrets.SecretRevisionMetadata

	if len(params.Revisions) == 0 {
		// Remove all revisions
		_, revs, err := s.secretState.GetSecretByURI(ctx, *uri, nil)
		if err != nil {
			return nil, errors.Capture(err)
		}
		s.logger.Tracef(ctx, "deleting all revisions for secret %q, found %d revisions", uri.ID, len(revs))
	} else {
		// Remove specified revisions
		revs = make([]*secrets.SecretRevisionMetadata, 0, len(params.Revisions))
		for _, rev := range params.Revisions {
			_, revsMeta, err := s.secretState.GetSecretByURI(ctx, *uri, &rev)
			if errors.Is(err, secreterrors.SecretRevisionNotFound) {
				continue
			}
			if err != nil {
				return nil, errors.Capture(err)
			}
			revs = append(revs, revsMeta...)
		}

		s.logger.Debugf(ctx, "deleting revisions %v for secret %q", params.Revisions, uri.ID)

		if len(revs) == 0 {
			return nil, errors2.NotFoundf("cannot delete any of revisions %v - revisions", params.Revisions)
		}
	}
	return revs, nil
}

// getObsoleteRevisionsForDeletion retrieves the metadata for obsolete revisions ready to be pruned.
func (s *SecretService) getObsoleteRevisionsForDeletion(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error) {
	// Get the list of obsolete revision IDs.
	obsoleteRevIDs, err := s.secretState.GetObsoleteUserSecretRevisionsReadyToPrune(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	s.logger.Tracef(ctx, "obsolete revisions ready to prune: %+v", obsoleteRevIDs)

	if len(obsoleteRevIDs) == 0 {
		return nil, nil
	}

	// Parse the revision IDs to get URIs and revision numbers.
	// Format is "secret-id/revision" e.g. "abc123/1"
	revisionsByURI := make(map[string][]int)
	for _, revID := range obsoleteRevIDs {
		lastSlash := -1
		for i := len(revID) - 1; i >= 0; i-- {
			if revID[i] == '/' {
				lastSlash = i
				break
			}
		}
		if lastSlash == -1 {
			return nil, errors.Errorf("invalid revision ID format: %q", revID)
		}
		secretID := revID[:lastSlash]
		revNum := 0
		_, err := fmt.Sscanf(revID[lastSlash+1:], "%d", &revNum)
		if err != nil {
			return nil, errors.Errorf("invalid revision number in %q: %w", revID, err)
		}
		revisionsByURI[secretID] = append(revisionsByURI[secretID], revNum)
	}

	// Fetch full metadata for each secret URI and its revisions.
	var result []*secrets.SecretMetadataForDrain
	for secretID, revisions := range revisionsByURI {
		uri, err := secrets.ParseURI(secretID)
		if err != nil {
			return nil, errors.Errorf("invalid secret URI %q: %w", secretID, err)
		}

		item := &secrets.SecretMetadataForDrain{
			URI:       uri,
			Revisions: make([]secrets.SecretExternalRevision, 0, len(revisions)),
		}

		for _, revNum := range revisions {
			_, revsMeta, err := s.secretState.GetSecretByURI(ctx, *uri, &revNum)
			if errors.Is(err, secreterrors.SecretRevisionNotFound) {
				// Revision was already deleted, skip it.
				s.logger.Debugf(ctx, "obsolete revision %s/%d no longer exists, skipping", uri.ID, revNum)
				continue
			}
			if err != nil {
				return nil, errors.Errorf("getting metadata for secret %s revision %d: %w", uri.ID, revNum, err)
			}

			if len(revsMeta) > 0 {
				item.Revisions = append(item.Revisions, secrets.SecretExternalRevision{
					Revision: revsMeta[0].Revision,
					ValueRef: revsMeta[0].ValueRef,
				})
			}
		}

		if len(item.Revisions) > 0 {
			result = append(result, item)
		}
	}

	return result, nil
}

func (s *SecretService) deleteObsoleteRevisionsFromBackend(
	ctx context.Context,
	obsoleteRevs []*secrets.SecretMetadataForDrain,
	_ map[string]provider.SecretRevisions,
) error {
	modelUUID, err := s.secretState.GetModelUUID(ctx)
	if err != nil {
		return errors.Errorf("getting model UUID: %w", err)
	}
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   string(modelUUID),
	}

	backend, backendID, err := s.getBackendForUserSecrets(ctx, accessor)
	if err != nil {
		return errors.Capture(err)
	}

	for _, item := range obsoleteRevs {
		r := make([]*secrets.SecretRevisionMetadata, len(item.Revisions))
		for i, v := range item.Revisions {
			r[i] = &secrets.SecretRevisionMetadata{
				Revision: v.Revision,
				ValueRef: v.ValueRef,
			}
		}

		err = s.deleteRevisionsFromBackend(ctx, item.URI, r, backend, backendID)
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (s *SecretService) deleteRevisionsFromBackend(
	ctx context.Context,
	uri *secrets.URI,
	revisions []*secrets.SecretRevisionMetadata,
	backend provider.SecretsBackend,
	backendID string,
) error {
	var err error
	providersToCleanUp := make(map[string]provider.SecretRevisions)

	for _, rev := range revisions {
		if rev.ValueRef == nil {
			// Internal secret. Nothing to do here.
			s.logger.Tracef(ctx, "cannot delete internal secret revision %q", rev.ValueRef.RevisionID)
			continue
		}

		revisionId := rev.ValueRef.RevisionID

		s.logger.Tracef(ctx, "deleting revision %q for secret %q", revisionId, uri.ID)
		s.logger.Tracef(ctx, "deleting revision %+v", rev)

		// Repeatedly attempt to delete the revision from the backend until one of:
		// * deletion is successful
		// * deletion is unsuccessful with error == NotFound, but the revision has not been moved to a new backend
		// * deletion is unsuccessful with error != NotFound
		for {
			if err != nil {
				return errors.Capture(err)
			}
			err = backend.DeleteContent(context.TODO(), revisionId)
			if err == nil {
				s.logger.Debugf(ctx, "deleted revision %+v", rev)
				// Deletion successful. Schedule this revision to be cleaned up in the provider and go to next revision.
				if _, ok := providersToCleanUp[backendID]; !ok {
					providersToCleanUp[backendID] = provider.SecretRevisions{}
				}
				providersToCleanUp[backendID].Add(uri, rev.ValueRef.RevisionID)
				break
			} else if !errors.Is(err, secreterrors.SecretRevisionNotFound) {
				// Exit early for any error other than NotFound.
				return errors2.Annotatef(err, "cannot remove revision %q from secret backend", revisionId)
			}

			s.logger.Tracef(ctx, "revision %q not found", revisionId)

			// NotFound could be because:
			// 1. The backend is draining and the secret was moved to the new backend before we accessed it.
			// 2. The secret is actually missing from the backend.
			// Check if the revision has moved to a new backend.
			_, revs, err := s.secretState.GetSecretByURI(ctx, *uri, &rev.Revision)
			if errors.Is(err, secreterrors.SecretRevisionNotFound) {
				// Revision no longer exists. Continue to the next.
				break
			}
			if err != nil {
				return errors2.Annotatef(err, "cannot get revision metadata for secret revision %s/%d", uri, rev.Revision)
			}
			updatedRev := revs[0]

			// If the backend changed, try to delete the secret from the new backend.
			if backendID != updatedRev.ValueRef.BackendID {
				backendID = updatedRev.ValueRef.BackendID
				revisionId = updatedRev.ValueRef.RevisionID
				continue
			}

			// Otherwise, the revision really is missing from the backend and we move on.
			// We tolerate this because our goal is to have that revision removed anyway.
			break
		}
	}
	return nil
}

func (s *SecretService) mapRevisions(revs []*secrets.SecretRevisionMetadata) {
	ser := make([]secrets.SecretExternalRevision, 0, len(revs))
	for _, rev := range revs {
		ser = append(ser, secrets.SecretExternalRevision{
			Revision: rev.Revision,
			ValueRef: rev.ValueRef,
		})
	}
}
