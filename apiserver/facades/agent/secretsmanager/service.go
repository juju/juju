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
	"github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretTriggers instances provide secret rotation/expiry apis.
type SecretTriggers interface {
	// WatchSecretRevisionsExpiryChanges returns a watcher that notifies when the
	// expiry time of a secret revision changes for the specified owners.
	WatchSecretRevisionsExpiryChanges(ctx context.Context, owners ...secret.CharmSecretOwner) (watcher.SecretTriggerWatcher, error)

	// WatchSecretsRotationChanges returns a watcher that reports when
	// secrets are due for rotation for the specified owners.
	WatchSecretsRotationChanges(ctx context.Context, owners ...secret.CharmSecretOwner) (watcher.SecretTriggerWatcher, error)

	// WatchObsoleteSecrets returns a watcher that reports when secret
	// revisions become obsolete for the specified owners.
	WatchObsoleteSecrets(ctx context.Context, owners ...secret.CharmSecretOwner) (watcher.StringsWatcher, error)

	// WatchDeletedSecrets returns a watcher that reports when secrets
	// are deleted for the specified owners.
	WatchDeletedSecrets(ctx context.Context, owners ...secret.CharmSecretOwner) (watcher.StringsWatcher, error)

	// SecretRotated records that a secret has been rotated at the given
	// time and schedules the next rotation.
	SecretRotated(ctx context.Context, uri *secrets.URI, params secretservice.SecretRotatedParams) error
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	// GetSecretConsumer returns the consumer metadata for a unit's
	// relationship with a specific secret.
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name) (*secrets.SecretConsumerMetadata, error)

	// GetSecretConsumerAndLatest returns the consumer metadata and the
	// latest revision for a specific secret.
	GetSecretConsumerAndLatest(ctx context.Context, uri *secrets.URI, unitName unit.Name) (*secrets.SecretConsumerMetadata, int, error)

	// GetURIByConsumerLabel returns the secret URI for the secret that
	// has the given label for the specified unit.
	GetURIByConsumerLabel(ctx context.Context, label string, unitName unit.Name) (*secrets.URI, error)

	// GetConsumedRevision returns the revision that a unit should
	// consume for the specified secret. The refresh and peek flags
	// control whether the unit advances to the latest revision; if a
	// label is provided via labelToUpdate, it is assigned to the
	// consumer's record.
	GetConsumedRevision(
		ctx context.Context, uri *secrets.URI, unitName unit.Name,
		refresh, peek bool, labelToUpdate *string) (int, error)

	// WatchConsumedSecretsChanges returns a watcher that reports
	// changes to secrets consumed by the specified unit.
	WatchConsumedSecretsChanges(ctx context.Context, unitName unit.Name) (watcher.StringsWatcher, error)
}

// SecretService provides core secrets operations.
type SecretService interface {
	// CreateSecretURIs generates the requested number of new secret URIs,
	// reserving them for the given accessor.
	CreateSecretURIs(ctx context.Context, accessor secret.SecretAccessor, count int) ([]*secrets.URI, error)

	// GetReservedSecretIDs returns the IDs of secrets that have been
	// reserved by the given accessor but not yet persisted.
	GetReservedSecretIDs(ctx context.Context, accessor secret.SecretAccessor) ([]string, error)

	// GetSecretValue retrieves the value and optional value reference for
	// the specified secret revision, checking access for the given accessor.
	GetSecretValue(context.Context, *secrets.URI, int, secret.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)

	// ListCharmSecrets returns the metadata and revision metadata for all
	// charm-owned secrets matching the given owners.
	ListCharmSecrets(context.Context, ...secret.CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	// ProcessCharmSecretConsumerLabel resolves a consumer label for a
	// charm secret, returning the URI and any previous label for the
	// consumer.
	ProcessCharmSecretConsumerLabel(
		ctx context.Context, unitName unit.Name, uri *secrets.URI, label string,
	) (*secrets.URI, *string, error)

	// ChangeSecretBackend migrates a secret revision to a different
	// backend, replacing content or value reference.
	ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params secretservice.ChangeSecretBackendParams) error

	// GetSecretGrants returns all access grants for a secret at the
	// specified role level.
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]secretservice.SecretAccess, error)

	// ListGrantedSecretsForBackend returns all secrets granted to the
	// given consumers at the specified role for the given backend.
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secret.SecretAccessor,
	) ([]*secrets.SecretRevisionRef, error)
}

// SecretBackendService provides access to the secret backend service.
type SecretBackendService interface {
	// DrainBackendConfigInfo returns the backend config for draining
	// secrets from the old backend to the new one.
	DrainBackendConfigInfo(
		ctx context.Context, p secretbackendservice.DrainBackendConfigParams,
	) (*provider.ModelBackendConfigInfo, error)

	// BackendConfigInfo returns the backend config for the given
	// parameters.
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
	// SaveRemoteSecretConsumer saves the consumer metadata for the given remote secret and unit.
	SaveRemoteSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name, md secrets.SecretConsumerMetadata,
		applicationUUID coreapplication.UUID, relationUUID corerelation.UUID) error
}
