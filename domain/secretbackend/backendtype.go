// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

// BackendType represents the type of a secret backend
// as recorded in the secret_backend_type lookup table.
type BackendType int

const (
	BackendTypeController BackendType = iota
	BackendTypeKubernetes
	BackendTypeVault
)

// MarshallBackendType converts a secret backend type to a db backend type id.
func MarshallBackendType(backendType string) (BackendType, error) {
	switch backendType {
	case juju.BackendType:
		return BackendTypeController, nil
	case kubernetes.BackendType:
		return BackendTypeKubernetes, nil
	case vault.BackendType:
		return BackendTypeVault, nil
	}
	return 0, errors.Errorf("secret backend type %q %w", backendType, coreerrors.NotValid)
}
