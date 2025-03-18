// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/uuid"
)

// AtomicState describes retrieval and persistence methods for
// secrets that require atomic transactions.
type AtomicState interface {
	domain.AtomicStateBase

	DeleteSecret(ctx domain.AtomicContext, uri *secrets.URI, revs []int) error
	GetSecretsForOwners(
		ctx domain.AtomicContext, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.URI, error)

	GetApplicationUUID(ctx domain.AtomicContext, appName string) (coreapplication.ID, error)
	GetUnitUUID(ctx domain.AtomicContext, name coreunit.Name) (coreunit.UUID, error)
	GetSecretOwner(ctx domain.AtomicContext, uri *secrets.URI) (domainsecret.Owner, error)

	CheckUserSecretLabelExists(ctx domain.AtomicContext, label string) (bool, error)
	CheckApplicationSecretLabelExists(ctx domain.AtomicContext, appUUID coreapplication.ID, label string) (bool, error)
	CheckUnitSecretLabelExists(ctx domain.AtomicContext, unitUUID coreunit.UUID, label string) (bool, error)
	CreateUserSecret(
		ctx domain.AtomicContext, version int, uri *secrets.URI, secret domainsecret.UpsertSecretParams,
	) error
	CreateCharmApplicationSecret(
		ctx domain.AtomicContext, version int, uri *secrets.URI, appUUID coreapplication.ID, secret domainsecret.UpsertSecretParams,
	) error
	CreateCharmUnitSecret(
		ctx domain.AtomicContext, version int, uri *secrets.URI, unitUUID coreunit.UUID, secret domainsecret.UpsertSecretParams,
	) error
	UpdateSecret(ctx domain.AtomicContext, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error
}

// State describes retrieval and persistence methods needed for
// the secrets domain service.
type State interface {
	AtomicState

	GetModelUUID(ctx context.Context) (coremodel.UUID, error)
	DeleteObsoleteUserSecretRevisions(ctx context.Context) ([]string, error)
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetLatestRevision(ctx context.Context, uri *secrets.URI) (int, error)
	GetSecretValue(ctx context.Context, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revision *int, labels domainsecret.Labels,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListCharmSecrets(ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name, md *secrets.SecretConsumerMetadata) error
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)
	GetURIByConsumerLabel(ctx context.Context, label string, unitName coreunit.Name) (*secrets.URI, error)
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name) (*secrets.SecretConsumerMetadata, int, error)
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name, md *secrets.SecretConsumerMetadata) error
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
	GetSecretRevisionID(ctx context.Context, uri *secrets.URI, revision int) (string, error)
	ChangeSecretBackend(
		ctx context.Context, revisionID uuid.UUID, valueRef *secrets.ValueRef, data secrets.SecretData,
	) error

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
	InitialWatchStatementForConsumedSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery)
	GetConsumedSecretURIsWithChanges(ctx context.Context, unitName coreunit.Name, revisionIDs ...string) ([]string, error)

	// For watching consumed remote secret changes.
	InitialWatchStatementForConsumedRemoteSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery)
	GetConsumedRemoteSecretURIsWithChanges(ctx context.Context, unitName coreunit.Name, secretIDs ...string) (secretURIs []string, err error)

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

	// NamespaceForWatchSecretMetadata returns namespace identifier for
	// secret metadata watcher.
	NamespaceForWatchSecretMetadata() string

	// NamespaceForWatchSecretRevisionObsolete returns namespace identifier for
	// obsolete secret revision watcher.
	NamespaceForWatchSecretRevisionObsolete() string
}

// SecretBackendReferenceMutator describes methods
// for modifying secret back-end references.
type SecretBackendReferenceMutator interface {
	// AddSecretBackendReference adds a reference to the
	// secret backend for the given secret revision.
	AddSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string,
	) (func() error, error)

	// RemoveSecretBackendReference removes the reference
	// to the secret backend for the given secret revision.
	RemoveSecretBackendReference(ctx context.Context, revisionIDs ...string) error

	// UpdateSecretBackendReference updates the reference
	// to the secret backend for the given secret revision.
	UpdateSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string,
	) (func() error, error)
}

// SecretBackendState describes persistence methods for working
// with secret backends in the controller database.
type SecretBackendState interface {
	SecretBackendReferenceMutator

	// GetModelSecretBackendDetails returns the details of the secret
	// backend that the input model is configured to use.
	GetModelSecretBackendDetails(
		ctx context.Context, modelUUID coremodel.UUID,
	) (secretbackend.ModelSecretBackend, error)

	// ListSecretBackendsForModel returns a list of all secret backends that
	// contain secrets for the specified model, unless includeEmpty is true,
	// in which case all backends are returned.
	ListSecretBackendsForModel(
		ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool,
	) ([]*secretbackend.SecretBackend, error)

	// GetActiveModelSecretBackend returns the active secret backend ID and config for the given model.
	// It returns an error satisfying [modelerrors.NotFound] if the model provided does not exist.
	GetActiveModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, *provider.ModelBackendConfig, error)
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
