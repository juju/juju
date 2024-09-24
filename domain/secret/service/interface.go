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
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/uuid"
)

// AtomicState describes retrieval and persistence methods for
// secrets that require atomic transactions.
type AtomicState interface {
	domain.AtomicStateBase

	ListExternalSecretRevisions(ctx domain.AtomicContext, uri *secrets.URI, revisions ...int) ([]secrets.ValueRef, error)
	DeleteSecret(ctx domain.AtomicContext, uri *secrets.URI, revs []int) ([]string, error)
	UpdateSecret(ctx domain.AtomicContext, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error
	GetSecretsForOwners(
		ctx domain.AtomicContext, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.URI, error)
	GetSecretValue(ctx domain.AtomicContext, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)
	GetSecretConsumer(ctx domain.AtomicContext, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretConsumer(ctx domain.AtomicContext, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	GetSecretRemoteConsumer(ctx domain.AtomicContext, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretRemoteConsumer(ctx domain.AtomicContext, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error

	GetSecretAccess(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.AccessParams) (string, error)
	GrantAccess(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.GrantParams) error
	RevokeAccess(ctx domain.AtomicContext, uri *secrets.URI, params domainsecret.AccessParams) error

	GetRotationExpiryInfo(ctx domain.AtomicContext, uri *secrets.URI) (*domainsecret.RotationExpiryInfo, error)
	GetRotatePolicy(ctx domain.AtomicContext, uri *secrets.URI) (secrets.RotatePolicy, error)
	SecretRotated(ctx domain.AtomicContext, uri *secrets.URI, next time.Time) error

	ListCharmSecrets(ctx domain.AtomicContext,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*domainsecret.SecretMetadata, error)

	ChangeSecretBackend(
		ctx domain.AtomicContext, revisionID uuid.UUID, valueRef *secrets.ValueRef, data secrets.SecretData,
	) error
}

// State describes retrieval and persistence methods needed for
// the secrets domain service.
type State interface {
	AtomicState

	GetModelUUID(ctx context.Context) (string, error)
	CreateUserSecret(
		ctx context.Context, version int, uri *secrets.URI, secret domainsecret.UpsertSecretParams,
	) error
	CreateCharmApplicationSecret(
		ctx context.Context, version int, uri *secrets.URI, appName string, secret domainsecret.UpsertSecretParams,
	) error
	CreateCharmUnitSecret(
		ctx context.Context, version int, uri *secrets.URI, unitName string, secret domainsecret.UpsertSecretParams,
	) error
	DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error)
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetLatestRevision(ctx context.Context, uri *secrets.URI) (int, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revision *int, labels domainsecret.Labels,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListAllSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*domainsecret.SecretRevisionMetadata, error)
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)
	GetURIByConsumerLabel(ctx context.Context, label string, unitName string) (*secrets.URI, error)
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (*domainsecret.AccessScope, error)
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]domainsecret.GrantParams, error)
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, accessors []domainsecret.AccessParams, role secrets.SecretRole,
	) ([]*secrets.SecretRevisionRef, error)
	ListCharmSecretsWithRevisions(ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListCharmSecretsToDrain(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadataForDrain, error)
	ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error)
	GetSecretRevisionID(ctx context.Context, uri *secrets.URI, revision int) (string, error)

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

	// Methods for loading secrets to be exported.
	AllSecretGrants(ctx context.Context) (map[string][]domainsecret.GrantParams, error)
	AllSecretConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error)
	AllSecretRemoteConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error)
	AllRemoteSecrets(ctx context.Context) ([]domainsecret.RemoteSecretInfo, error)
}

// SecretBackendReferenceMutator describes methods for interacting with the secret backend state.
type SecretBackendReferenceMutator interface {
	// AddSecretBackendReference adds a reference to the secret backend for the given secret revision.
	AddSecretBackendReference(ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string) (func() error, error)
	// RemoveSecretBackendReference removes the reference to the secret backend for the given secret revision.
	RemoveSecretBackendReference(ctx context.Context, revisionIDs ...string) error
	// UpdateSecretBackendReference updates the reference to the secret backend for the given secret revision.
	UpdateSecretBackendReference(ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string) (func() error, error)
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
