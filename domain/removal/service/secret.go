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

type SecretModelState interface {
	// DeleteApplicationOwnedSecretContent deletes content for all
	// secrets owned by the application with the input UUID.
	// It must only be called in the context of application removal.
	DeleteApplicationOwnedSecretContent(ctx context.Context, aUUID string) error

	// DeleteUnitOwnedSecretContent deletes content for all
	// secrets owned by the unit with the input UUID.
	// It must only be called in the context of unit removal.
	DeleteUnitOwnedSecretContent(ctx context.Context, uUUID string) error
}

func (s *Service) deleteApplicationOwnedSecrets(ctx context.Context, aUUID coreapplication.UUID) error {
	sb, err := s.getSecretBackend(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if sb == nil {
		return errors.Capture(s.modelState.DeleteApplicationOwnedSecretContent(ctx, aUUID.String()))
	}

	// TODO: Use the secret back-end to remove secrets.
	return nil
}

func (s *Service) deleteUnitOwnedSecrets(ctx context.Context, uUUID coreunit.UUID) error {
	sb, err := s.getSecretBackend(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if sb == nil {
		return errors.Capture(s.modelState.DeleteUnitOwnedSecretContent(ctx, uUUID.String()))
	}

	// TODO: Use the secret back-end to remove secrets.
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
