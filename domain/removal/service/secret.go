// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	coreunit "github.com/juju/juju/core/unit"
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
				s.logger.Warningf(ctx, "failed to delete secret content for external reference %q: %s", id, err.Error())
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
				s.logger.Warningf(ctx, "failed to delete secret content for external reference %q: %s", id, err.Error())
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
