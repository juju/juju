// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func (s *State) getModelSecretBackendDetailsForTest(ctx context.Context, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error) {
	var modelSecretBackend secretbackend.ModelSecretBackend
	err := s.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		modelSecretBackend, err = s.GetModelSecretBackendDetails(ctx, uuid)
		return err
	})
	return modelSecretBackend, err
}

func (s *State) getSecretBackendForTest(ctx context.Context, params secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error) {
	var secretBackend *secretbackend.SecretBackend
	err := s.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		secretBackend, err = s.GetSecretBackend(ctx, params)
		return err
	})
	return secretBackend, err
}

func (s *State) listSecretBackendsForModelForTest(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error) {
	var secretBackends []*secretbackend.SecretBackend
	err := s.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		secretBackends, err = s.ListSecretBackendsForModel(ctx, modelUUID, includeEmpty)
		return err
	})
	return secretBackends, err
}

func (s *State) setModelSecretBackendForTest(ctx context.Context, modelUUID coremodel.UUID, secretBackendName string) error {
	return s.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return s.SetModelSecretBackend(ctx, modelUUID, secretBackendName)
	})
}
