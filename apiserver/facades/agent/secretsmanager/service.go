// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"

	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretTriggers instances provide secret rotation/expiry apis.
type SecretTriggers interface {
	WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...secretservice.CharmSecretOwner) (watcher.SecretTriggerWatcher, error)
	WatchSecretsRotationChanges(ctx context.Context, owners ...secretservice.CharmSecretOwner) (watcher.SecretTriggerWatcher, error)
	WatchObsoleteSecrets(ctx context.Context, owners ...secretservice.CharmSecretOwner) (watcher.StringsWatcher, error)
	WatchDeletedSecrets(ctx context.Context, owners ...secretservice.CharmSecretOwner) (watcher.StringsWatcher, error)
	SecretRotated(ctx context.Context, uri *secrets.URI, params secretservice.SecretRotatedParams) error
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name) (*secrets.SecretConsumerMetadata, error)
	GetSecretConsumerAndLatest(ctx context.Context, uri *secrets.URI, unitName unit.Name) (*secrets.SecretConsumerMetadata, int, error)
	GetURIByConsumerLabel(ctx context.Context, label string, unitName unit.Name) (*secrets.URI, error)
	SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name, md secrets.SecretConsumerMetadata) error
	GetConsumedRevision(
		ctx context.Context, uri *secrets.URI, unitName unit.Name,
		refresh, peek bool, labelToUpdate *string) (int, error)
	WatchConsumedSecretsChanges(ctx context.Context, unitName unit.Name) (watcher.StringsWatcher, error)
	GrantSecretAccess(context.Context, *secrets.URI, secretservice.SecretAccessParams) error
	RevokeSecretAccess(context.Context, *secrets.URI, secretservice.SecretAccessParams) error
}

// SecretService provides core secrets operations.
type SecretService interface {
	CreateSecretURIs(ctx context.Context, count int) ([]*secrets.URI, error)
	GetSecretValue(context.Context, *secrets.URI, int, secretservice.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)
	ListCharmSecrets(context.Context, ...secretservice.CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ProcessCharmSecretConsumerLabel(
		ctx context.Context, unitName unit.Name, uri *secrets.URI, label string,
	) (*secrets.URI, *string, error)
	ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params secretservice.ChangeSecretBackendParams) error
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]secretservice.SecretAccess, error)
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

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationUUIDByName returns an application UUID by application name.
	GetApplicationUUIDByName(ctx context.Context, name string) (coreapplication.UUID, error)
}

// CrossModelRelationService provides access to the cross model relation service.
type CrossModelRelationService interface {
	// GetMacaroonForRelation gets the given macaroon for the specified remote relation.
	GetMacaroonForRelation(ctx context.Context, relationUUID corerelation.UUID) (*macaroon.Macaroon, error)
}
