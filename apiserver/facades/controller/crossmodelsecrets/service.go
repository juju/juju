// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"

	"github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	GetSecret(context.Context, *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(context.Context, *secrets.URI, int, secretservice.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, accessor secretservice.SecretAccessor) (secretservice.SecretAccessScope, error)
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secretservice.SecretAccessor,
	) ([]*secrets.SecretRevisionRef, error)
}

// SecretBackendService provides access to the secret backend service,
type SecretBackendService interface {
	BackendConfigInfo(
		ctx context.Context, p secretbackendservice.BackendConfigParams,
	) (*provider.ModelBackendConfigInfo, error)
}
