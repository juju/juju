// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// The following interfaces are used to access secret services.

// SecretService provides access to the secret service,
type SecretService interface {
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

// ApplicationService provides access to the application service,
type ApplicationService interface {
	// GetApplicationName returns the name of the specified application.
	GetApplicationName(ctx context.Context, appID coreapplication.UUID) (string, error)
}

// CrossModelRelationService provides access to the cross model relation service,
type CrossModelRelationService interface {
	// ProcessRemoteConsumerGetSecret returns the content of a remotely consumed secret,
	// and the latest secret revision.
	ProcessRemoteConsumerGetSecret(
		ctx context.Context, uri *secrets.URI, unitName unit.Name, revision *int, peek, refresh bool,
	) (secrets.SecretValue, *secrets.ValueRef, int, error)

	// IsCrossModelRelationValidForApplication checks that the cross model relation is valid for the application.
	// A relation is valid if it is not suspended and the application is involved in the relation.
	IsCrossModelRelationValidForApplication(ctx context.Context, key corerelation.Key, appName string) (bool, error)
}
