// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider"
)

// DeleteObsoleteUserSecretRevisions deletes any obsolete user secret revisions that are marked as auto-prune.
func (s *SecretService) DeleteObsoleteUserSecretRevisions(ctx context.Context) error {
	return s.st.DeleteObsoleteUserSecretRevisions(ctx)
}

// DeleteSecret removes the specified secret.
// If revisions is nil or the last remaining revisions are removed.
// It returns [secreterrors.PermissionDenied] if the secret cannot be managed by the accessor.
func (s *SecretService) DeleteSecret(ctx context.Context, uri *secrets.URI, params DeleteSecretParams) error {
	if err := s.canManage(ctx, uri, params.Accessor, params.LeaderToken); err != nil {
		return errors.Trace(err)
	}

	// We remove the secret from the backend first.
	if err := s.removeFromExternal(ctx, uri, params.Accessor, params.Revisions...); err != nil {
		return errors.Trace(err)
	}

	return s.st.DeleteSecret(ctx, uri, params.Revisions)
}

func (s *SecretService) removeFromExternal(ctx context.Context, uri *secrets.URI, accessor SecretAccessor, revisions ...int) error {
	externalRevs := make(map[string]provider.SecretRevisions)
	revs, err := s.st.ListExternalSecretRevisions(ctx, uri, revisions...)
	if err != nil {
		return errors.Trace(err)
	}
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
		p, err := s.providerGetter(backendCfg.BackendType)
		if err != nil {
			return errors.Trace(err)
		}
		if err := s.removeFromBackend(ctx, p, backendCfg, accessor, r); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (s *SecretService) removeFromBackend(
	ctx context.Context, p provider.SecretBackendProvider, cfg provider.ModelBackendConfig,
	accessor SecretAccessor, revs provider.SecretRevisions,
) error {
	backend, err := p.NewBackend(&cfg)
	if err != nil {
		return errors.Trace(err)
	}
	// For models, we need to delete the content.
	if accessor.Kind == ModelAccessor {
		for _, revId := range revs.RevisionIDs() {
			if err = backend.DeleteContent(ctx, revId); err != nil && !errors.Is(err, secreterrors.SecretRevisionNotFound) {
				return errors.Annotatef(err, "deleting secret content from backend for %q", revId)
			}
		}
	}
	// For units, we want to clean up any backend artefacts.
	if accessor.Kind == UnitAccessor {
		if err := p.CleanupSecrets(ctx, &cfg, accessor.ID, revs); err != nil {
			return errors.Annotatef(err, "cleaning secret resources from %s backend for unit %q", p.Type(), accessor.ID)
		}
	}
	return nil
}
