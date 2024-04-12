// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"

	coresecrets "github.com/juju/juju/core/secrets"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
)

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	CreateSecretBackend(context.Context, coresecrets.SecretBackend) error
	UpdateSecretBackend(context.Context, secretbackendservice.UpdateSecretBackendParams) error
	DeleteSecretBackend(context.Context, secretbackendservice.DeleteSecretBackendParams) error
	GetSecretBackendByName(context.Context, string) (*coresecrets.SecretBackend, error)

	BackendSummaryInfo(ctx context.Context, reveal, all bool, names ...string) ([]*secretbackendservice.SecretBackendInfo, error)
}
