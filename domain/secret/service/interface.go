// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainsecret "github.com/juju/juju/domain/secret"
)

// State describes retrieval and persistence methods needed for
// the secrets domain service.
type State interface {
	GetModelUUID(ctx context.Context) (string, error)
	CreateUserSecret(
		ctx context.Context, version int, uri *secrets.URI, secret domainsecret.UpsertSecretParams,
	) (string, error)
	CreateCharmApplicationSecret(
		ctx context.Context, version int, uri *secrets.URI, appName string, secret domainsecret.UpsertSecretParams,
	) (string, error)
	CreateCharmUnitSecret(
		ctx context.Context, version int, uri *secrets.URI, unitName string, secret domainsecret.UpsertSecretParams,
	) (string, error)
	UpdateSecret(ctx context.Context, uri *secrets.URI, secret domainsecret.UpsertSecretParams) (string, error)
	DeleteSecret(ctx context.Context, uri *secrets.URI, revs []int) ([]string, error)
	DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error)
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetLatestRevision(ctx context.Context, uri *secrets.URI) (int, error)
	ListExternalSecretRevisions(ctx context.Context, uri *secrets.URI, revisions ...int) ([]secrets.ValueRef, error)
	GetSecretValue(ctx context.Context, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revision *int, labels domainsecret.Labels,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListCharmSecrets(ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)
	GetURIByConsumerLabel(ctx context.Context, label string, unitName string) (*secrets.URI, error)
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error
	GrantAccess(ctx context.Context, uri *secrets.URI, params domainsecret.GrantParams) error
	RevokeAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) error
	GetSecretAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (string, error)
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (*domainsecret.AccessScope, error)
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]domainsecret.GrantParams, error)
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, accessors []domainsecret.AccessParams, role secrets.SecretRole,
	) ([]*secrets.SecretRevisionRef, error)
	ListCharmSecretsToDrain(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadataForDrain, error)
	ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error)
	SecretRotated(ctx context.Context, uri *secrets.URI, next time.Time) error
	GetRotatePolicy(ctx context.Context, uri *secrets.URI) (secrets.RotatePolicy, error)
	GetRotationExpiryInfo(ctx context.Context, uri *secrets.URI) (*domainsecret.RotationExpiryInfo, error)
	ChangeSecretBackend(
		ctx context.Context, uri *secrets.URI, revision int, valueRef *secrets.ValueRef, data secrets.SecretData,
	) (string, error)

	// For watching obsolete secret revision changes.
	InitialWatchStatementForObsoleteRevision(
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (tableName string, statement eventsource.NamespaceQuery)
	GetRevisionIDsForObsolete(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string,
	) ([]string, error)

	// For watching obsolete user secret revisions to prune.
	GetObsoleteUserSecretRevisionsReadyToPrune(ctx context.Context) ([]string, error)

	// For watching consumed local secret changes.
	InitialWatchStatementForConsumedSecretsChange(unitName string) (string, eventsource.NamespaceQuery)
	GetConsumedSecretURIsWithChanges(ctx context.Context, unitName string, revisionIDs ...string) ([]string, error)

	// For watching consumed remote secret changes.
	InitialWatchStatementForConsumedRemoteSecretsChange(unitName string) (string, eventsource.NamespaceQuery)
	GetConsumedRemoteSecretURIsWithChanges(ctx context.Context, unitName string, secretIDs ...string) (secretURIs []string, err error)

	// For watching local secret changes that consumed by remote consumers.
	InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appName string) (string, eventsource.NamespaceQuery)
	GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx context.Context, appName string, secretIDs ...string) ([]string, error)

	// For watching secret rotation changes.
	InitialWatchStatementForSecretsRotationChanges(
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (string, eventsource.NamespaceQuery)
	GetSecretsRotationChanges(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, secretIDs ...string,
	) ([]domainsecret.RotationInfo, error)

	// For watching secret revision expiry changes.
	InitialWatchStatementForSecretsRevisionExpiryChanges(
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (string, eventsource.NamespaceQuery)
	GetSecretsRevisionExpiryChanges(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string,
	) ([]domainsecret.ExpiryInfo, error)
}

// SecretBackendReferenceMutator describes methods for interacting with the secret backend state.
type SecretBackendReferenceMutator interface {
	// AddSecretBackendReference adds a reference to the secret backend for the given secret revision.
	AddSecretBackendReference(ctx context.Context, backendID *string, modelID coremodel.UUID, revisionID string) error
	// RemoveSecretBackendReference removes the reference to the secret backend for the given secret revision.
	RemoveSecretBackendReference(ctx context.Context, revisionIDs ...string) error
	// UpdateSecretBackendReference updates the reference to the secret backend for the given secret revision.
	UpdateSecretBackendReference(ctx context.Context, backendID *string, modelID coremodel.UUID, revisionID string) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)

	// NewNamespaceNotifyMapperWatcher returns a new namespace notify watcher
	// for events based on the input change mask and mapper.
	NewNamespaceNotifyMapperWatcher(
		namespace string, changeMask changestream.ChangeType, mapper eventsource.Mapper,
	) (watcher.NotifyWatcher, error)
}
