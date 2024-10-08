// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
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
