// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider"
)

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	deletedRevisionIDs, err := s.secretState.DeleteObsoleteUserSecretRevisions(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err = s.secretBackendReferenceMutator.RemoveSecretBackendReference(ctx, deletedRevisionIDs...); err != nil {
		// We don't want to error out if we can't remove the backend reference.
		s.logger.Errorf("failed to remove secret backend reference for deleted obsolete user secret revisions: %v", err)
	}
	return nil
}

// DeleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params DeleteSecretParams) error {
	isLeader, err := checkLeaderToken(params.LeaderToken)
	if err != nil {
		return errors.Trace(err)
	}
	var cleanExternal func(context.Context)
	if err := s.secretState.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := s.canManage(ctx, isLeader, uri, params.Accessor)
		if err != nil {
			return errors.Trace(err)
		}
		cleanExternal, err = s.InternalDeleteSecret(ctx, uri, params)
		return errors.Trace(err)
	}); err != nil {
		return errors.Annotatef(err, "deleting secret %q", uri.ID)
	}
	// Delete an external content and remove references from the affected backend(s).
	cleanExternal(ctx)
	return nil
}

// InternalDeleteSecret removes the specified secret.
// If revisions is nil the last remaining revisions are removed.
// It is called by [DeleteSecret] on this service and also by the application service when deleting
// secrets owned by a unit or application being deleted. No permission checks are done.
// It returns a function which can be called to delete any external content and also remove
// any references from the affected backend(s).
func (s *SecretService) InternalDeleteSecret(ctx domain.AtomicContext, uri *secrets.URI, params DeleteSecretParams) (func(context.Context), error) {
	external, err := s.secretState.ListExternalSecretRevisions(ctx, uri, params.Revisions...)
	if err != nil {
		return nil, errors.Annotatef(err, "listing external revisions for %q", uri.ID)
	}

	deletedRevisionIDs, err := s.secretState.DeleteSecret(ctx, uri, params.Revisions)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return func(ctx context.Context) {
		// Remove any external secrets stored in a backend.
		if err := s.removeFromExternal(ctx, uri, params.Accessor, external); err != nil {
			// We don't want to error out if we can't remove the external content.
			s.logger.Errorf("removing external content for secret %q: %v", uri.ID, err)
		}

		// Backend references live in the controller db.
		if err := s.secretBackendReferenceMutator.RemoveSecretBackendReference(ctx, deletedRevisionIDs...); err != nil {
			// We don't want to error out if we can't remove the backend reference.
			s.logger.Errorf("failed to remove secret backend reference for deleted secret revisions %v: %v", deletedRevisionIDs, err)
		}
	}, nil
}

func (s *SecretService) removeFromExternal(ctx context.Context, uri *secrets.URI, accessor SecretAccessor, revs []secrets.ValueRef) error {
	externalRevs := make(map[string]provider.SecretRevisions)
	for _, valRef := range revs {
		if _, ok := externalRevs[valRef.BackendID]; !ok {
			externalRevs[valRef.BackendID] = provider.SecretRevisions{}
		}
		externalRevs[valRef.BackendID].Add(uri, valRef.RevisionID)
	}

	if len(externalRevs) == 0 {
		return nil
	}

	cfgInfo, err := s.adminConfigGetter(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	for backendID, r := range externalRevs {
		backendCfg, ok := cfgInfo.Configs[backendID]
		if !ok {
			return fmt.Errorf("secret backend %q not found%w", backendID, secretbackenderrors.NotFound)
		}
		if err := s.removeFromBackend(ctx, backendCfg, accessor, r); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (s *SecretService) removeFromBackend(
	ctx context.Context, cfg provider.ModelBackendConfig,
	accessor SecretAccessor, revs provider.SecretRevisions,
) error {
	p, err := s.providerGetter(cfg.BackendType)
	if err != nil {
		return errors.Trace(err)
	}
	backend, err := s.getBackend(&cfg)
	if err != nil {
		return errors.Trace(err)
	}

	for _, revId := range revs.RevisionIDs() {
		if err = backend.DeleteContent(ctx, revId); err != nil && !errors.Is(err, secreterrors.SecretRevisionNotFound) {
			return errors.Annotatef(err, "deleting secret content from backend for %q", revId)
		}
	}
	// For units, we want to clean up any backend artefacts.
	if accessor.Kind == UnitAccessor {
		if err := p.CleanupSecrets(ctx, &cfg, accessor.ID, revs); err != nil {
			return errors.Annotatef(err, "cleaning secret resources from %s backend for unit %q", cfg.BackendType, accessor.ID)
		}
	}
	return nil
}
