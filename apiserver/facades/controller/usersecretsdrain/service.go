// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"context"

	"github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretService provides access to the secret service.
type SecretService interface {
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(context.Context, *secrets.URI, int, secretservice.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secretservice.SecretAccessor,
	) ([]*secrets.SecretRevisionRef, error)
}

// SecretBackendService provides access to the secret backend service,
type SecretBackendService interface {
	DrainBackendConfigInfo(
		ctx context.Context, p secretbackendservice.DrainBackendConfigParams,
	) (*provider.ModelBackendConfigInfo, error)
	BackendConfigInfo(
		ctx context.Context, p secretbackendservice.BackendConfigParams,
	) (*provider.ModelBackendConfigInfo, error)
}
