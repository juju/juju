// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for detailst.

package state

import (
	"context"
	"testing"

	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func getModelSecretBackendDetails(ctx context.Context, st *State, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error) {
	var modelSecretBackend secretbackend.ModelSecretBackend
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		modelSecretBackend, err = st.GetModelSecretBackendDetails(ctx, uuid)
		return err
	})
	return modelSecretBackend, err
}

func getSecretBackend(ctx context.Context, st *State, params secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error) {
	var secretBackend *secretbackend.SecretBackend
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		secretBackend, err = st.GetSecretBackend(ctx, params)
		return err
	})
	return secretBackend, err
}

func listSecretBackendsForModel(ctx context.Context, st *State, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error) {
	var secretBackends []*secretbackend.SecretBackend
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		secretBackends, err = st.ListSecretBackendsForModel(ctx, modelUUID, includeEmpty)
		return err
	})
	return secretBackends, err
}

func setModelSecretBackend(ctx context.Context, st *State, modelUUID coremodel.UUID, secretBackendName string) error {
	return st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.SetModelSecretBackend(ctx, modelUUID, secretBackendName)
	})
}
