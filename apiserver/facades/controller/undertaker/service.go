// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretBackendService defines the methods that the secret backend service
type SecretBackendService interface {
	// GetSecretBackendConfigForAdmin returns the secret backend configuration
	// for the given backend ID for an admin user.
	GetSecretBackendConfigForAdmin(ctx context.Context, modelUUID coremodel.UUID) (*provider.ModelBackendConfigInfo, error)
}
