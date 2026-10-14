// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/secret"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// The following interfaces are used to access secret services.

// SecretService provides access to the secret service,
type SecretService interface {
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secret.SecretAccessor,
	) ([]*secrets.SecretRevisionRef, error)
}

// SecretBackendService provides access to the secret backend service,
type SecretBackendService interface {
	BackendConfigInfo(
		ctx context.Context, p secretbackendservice.BackendConfigParams,
	) (*provider.ModelBackendConfigInfo, error)
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

	// GetRemoteConsumerSecretGrants returns the grants of the specified
	// secret to the remote consumer identified by the consuming application
	// UUID: one entry per offer connection of the consuming application
	// over which the secret was granted, ordered by the name of the
	// synthetic application representing the consuming application for
	// that connection.
	// It returns an error satisfying [secreterrors.SecretNotFound] if the
	// secret does not exist, and an error satisfying
	// [secreterrors.SecretAccessScopeNotFound] if the consuming application
	// holds no grant of the secret. Callers map these sentinel errors to
	// API errors: a missing secret is reported as not found, and a
	// missing access scope is either denied or reported as not found.
	GetRemoteConsumerSecretGrants(
		ctx context.Context, uri *secrets.URI, consumingAppUUID coreapplication.UUID,
	) ([]crossmodelrelation.RemoteConsumerSecretGrant, error)
}
