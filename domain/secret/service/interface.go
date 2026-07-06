// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods needed for
// the secrets domain service.
type State interface {
	// GetApplicationUUID returns the UUID for the application with the
	// given name.
	GetApplicationUUID(ctx context.Context, appName string) (coreapplication.UUID, error)

	// GetModelUUID returns the UUID of the current model.
	GetModelUUID(ctx context.Context) (coremodel.UUID, error)

	// GetUnitUUID returns the UUID for the unit with the given name.
	GetUnitUUID(ctx context.Context, name coreunit.Name) (coreunit.UUID, error)

	// ReserveSecretURIs records that the given secret IDs have been
	// minted for the specified unit but not yet persisted as charm
	// secrets. This allows backend write authority to be granted only
	// for IDs the requesting unit actually reserved.
	ReserveSecretURIs(ctx context.Context, unitUUID coreunit.UUID, secretIDs []string) error

	// GetUnitReservedSecretIDs returns the IDs of all secrets reserved
	// by the given unit that have not yet been persisted.
	GetUnitReservedSecretIDs(ctx context.Context, unitUUID coreunit.UUID) ([]string, error)

	// ImportSecretWithRevisions imports a secret with its revisions,
	// owner, and metadata into the model.
	ImportSecretWithRevisions(ctx context.Context, version int, uri *secrets.URI,
		owner domainsecret.Owner,
		metaParams domainsecret.UpsertSecretParams,
		revisions []domainsecret.UpsertRevisionParams) error

	// CreateUserSecret creates a new user-owned secret.
	CreateUserSecret(ctx context.Context, version int, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error

	// GetSecret returns metadata for the secret identified by URI.
	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)

	// GetSecretOwnerKinds returns the owner kind for each of the given
	// secret URIs. Secrets that no longer exist are silently omitted.
	GetSecretOwnerKinds(ctx context.Context, uris []*secrets.URI) ([]domainsecret.SecretOwnerInfo, error)

	// GetLatestRevision returns the latest revision number for the
	// specified secret.
	GetLatestRevision(ctx context.Context, uri *secrets.URI) (int, error)

	// GetLatestRevisions returns the latest revision for each of the
	// given secret URIs, keyed by URI ID.
	GetLatestRevisions(ctx context.Context, uris []*secrets.URI) (map[string]int, error)

	// GetSecretValue returns the data and optional value reference for
	// the specified secret revision.
	GetSecretValue(ctx context.Context, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)

	// GetSecretByURI returns the metadata and all revision metadata for
	// the specified secret. If revision is non-nil, only that revision's
	// metadata is returned.
	GetSecretByURI(ctx context.Context, uri secrets.URI, revision *int) (*secrets.SecretMetadata,
		[]*secrets.SecretRevisionMetadata, error)

	// ListSecretsByLabels returns secrets matching the given labels
	// and optional revision filter.
	ListSecretsByLabels(ctx context.Context, labels domainsecret.Labels, revision *int) ([]*secrets.SecretMetadata,
		[][]*secrets.SecretRevisionMetadata, error)

	// ListAllSecrets returns all secrets in the model.
	ListAllSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	// ListCharmSecrets returns all charm-owned secrets for the given
	// application and unit owners.
	ListCharmSecrets(ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	// GetSecretConsumer returns the consumer metadata and latest
	// revision for the specified secret and unit.
	GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name) (*secrets.SecretConsumerMetadata, int, error)

	// SaveSecretConsumer persists or updates the consumer metadata for
	// a unit's relationship with a specific secret.
	SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName coreunit.Name, md secrets.SecretConsumerMetadata) error

	// GetUserSecretURIByLabel returns the URI for the user secret with
	// the given label.
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)

	// GetURIByConsumerLabel returns the URI for the secret with the
	// given consumer label for the specified unit.
	GetURIByConsumerLabel(ctx context.Context, label string, unitName coreunit.Name) (*secrets.URI, error)

	// GrantAccess grants the specified access parameters on a secret.
	GrantAccess(ctx context.Context, uri *secrets.URI, params domainsecret.GrantParams) error

	// RevokeAccess revokes the specified access parameters on a secret.
	RevokeAccess(ctx context.Context, uri *secrets.URI, params domainsecret.RevokeParams) error

	// GetSecretAccess returns the access string for the secret given
	// the specified access parameters.
	GetSecretAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (string, error)

	// GetSecretAccessRelationScope returns the relation scope UUID for
	// the given secret access params, or an empty string if none.
	GetSecretAccessRelationScope(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (string, error)

	// GetRegularRelationUUIDByEndpointIdentifiers returns the UUID of a
	// relation matching the given endpoint identifiers.
	GetRegularRelationUUIDByEndpointIdentifiers(ctx context.Context, endpoint1, endpoint2 corerelation.EndpointIdentifier) (string, error)

	// GetRelationEndpoints returns the endpoint identifiers for the
	// relation with the given UUID.
	GetRelationEndpoints(ctx context.Context, relationUUID string) ([]corerelation.EndpointIdentifier, error)

	// GetSecretGrants returns all grants for the given secret at the
	// specified role level.
	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]domainsecret.GrantDetails, error)

	// ListGrantedSecretsForBackend returns all secrets granted to the
	// specified accessors at the given roles for the given backend.
	ListGrantedSecretsForBackend(
		ctx context.Context, backendID string, accessors []domainsecret.AccessParams, roles []domainsecret.Role,
	) ([]*secrets.SecretRevisionRef, error)

	// ListCharmSecretsToDrain returns charm secrets that are ready to
	// be drained from the old backend.
	ListCharmSecretsToDrain(
		ctx context.Context,
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) ([]*secrets.SecretMetadataForDrain, error)

	// ListUserSecretsToDrain returns user secrets that are ready to be
	// drained from the old backend.
	ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error)

	// SecretRotated records that a secret has been rotated and sets the
	// next rotation time.
	SecretRotated(ctx context.Context, uri *secrets.URI, next time.Time) error

	// GetRotationExpiryInfo returns rotation and expiry info for the
	// specified secret.
	GetRotationExpiryInfo(ctx context.Context, uri *secrets.URI) (*domainsecret.RotationExpiryInfo, error)

	// GetSecretRevisionUUID returns the revision UUID for a given
	// secret URI and revision number.
	GetSecretRevisionUUID(ctx context.Context, uri *secrets.URI, revision int) (string, error)

	// ChangeSecretBackend updates the backend reference for a secret
	// revision, replacing the value reference or data.
	ChangeSecretBackend(
		ctx context.Context, revisionID uuid.UUID, valueRef *secrets.ValueRef, data secrets.SecretData,
	) error

	// GetOwnedSecretIDs returns all secret IDs owned by the given
	// application and unit owner UUIDs.
	GetOwnedSecretIDs(
		ctx context.Context, appOwnerUUIDs []string, unitOwnerUUIDs []string,
	) ([]string, error)

	// GetApplicationUUIDsForNames returns the UUIDs for the given
	// application names.
	GetApplicationUUIDsForNames(ctx context.Context, names domainsecret.ApplicationOwners) ([]string, error)

	// GetUnitUUIDsForNames returns the UUIDs for the given unit names.
	GetUnitUUIDsForNames(ctx context.Context, names domainsecret.UnitOwners) ([]string, error)

	// UpdateSecret updates the metadata and/or content of an existing
	// secret.
	UpdateSecret(ctx context.Context, uri *secrets.URI, secret domainsecret.UpsertSecretParams) error

	// ScheduleUserSecretRemoval schedules a user secret for removal at
	// the specified time.
	ScheduleUserSecretRemoval(ctx context.Context, removalUUID string, uri *secrets.URI, revisions []int, when time.Time) error

	// ScheduleObsoleteUserSecretRevisionsPruning schedules pruning of
	// obsolete user secret revisions at the specified time.
	ScheduleObsoleteUserSecretRevisionsPruning(ctx context.Context, jobUUID string, when time.Time) error

	// InitialWatchStatementForObsoleteRevision returns the table name and
	// namespace query for watching obsolete secret revisions.
	InitialWatchStatementForObsoleteRevision(
		appOwnerUUIDs domainsecret.ApplicationOwners, unitOwnerUUIDs domainsecret.UnitOwners,
	) (tableName string, statement eventsource.NamespaceQuery)

	// GetRevisionIDsForObsolete returns the revision UUIDs that have
	// become obsolete for the given application and unit owners.
	GetRevisionIDsForObsolete(
		ctx context.Context, appUUIDs domainsecret.ApplicationOwners, unitUUIDs domainsecret.UnitOwners, revisionUUIDs []string,
	) ([]string, error)

	// GetObsoleteUserSecretRevisionsReadyToPrune returns revision UUIDs
	// for obsolete user secret revisions that are ready to be pruned.
	GetObsoleteUserSecretRevisionsReadyToPrune(ctx context.Context) ([]string, error)

	// InitialWatchStatementForConsumedSecretsChange returns the table
	// name and namespace query for watching consumed local secret changes.
	InitialWatchStatementForConsumedSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery)

	// GetConsumedSecretURIsWithChanges returns the URIs of local secrets
	// consumed by the given unit that have pending changes.
	GetConsumedSecretURIsWithChanges(ctx context.Context, unitName coreunit.Name, revisionIDs ...string) ([]string, error)

	// InitialWatchStatementForConsumedRemoteSecretsChange returns the
	// table name and namespace query for watching consumed remote secret changes.
	InitialWatchStatementForConsumedRemoteSecretsChange(unitName coreunit.Name) (string, eventsource.NamespaceQuery)

	// GetConsumedRemoteSecretURIsWithChanges returns the URIs of remote
	// secrets consumed by the given unit that have pending changes.
	GetConsumedRemoteSecretURIsWithChanges(ctx context.Context, unitName coreunit.Name, secretIDs ...string) (secretURIs []string, err error)

	// InitialWatchStatementForSecretsRotationChanges returns the table
	// name and namespace query for watching secret rotation changes.
	InitialWatchStatementForSecretsRotationChanges(
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (string, eventsource.NamespaceQuery)

	// GetSecretsRotationChanges returns rotation info for secrets
	// matching the given owner scopes and optional secret IDs.
	GetSecretsRotationChanges(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, secretIDs ...string,
	) ([]domainsecret.RotationInfo, error)

	// InitialWatchStatementForSecretsRevisionExpiryChanges returns the
	// table name and namespace query for watching secret revision expiry changes.
	InitialWatchStatementForSecretsRevisionExpiryChanges(
		appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners,
	) (string, eventsource.NamespaceQuery)

	// GetSecretsRevisionExpiryChanges returns expiry info for secret
	// revisions matching the given owner scopes and optional revision UUIDs.
	GetSecretsRevisionExpiryChanges(
		ctx context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string,
	) ([]domainsecret.ExpiryInfo, error)

	// AllSecretGrants returns all secret grants keyed by secret ID.
	AllSecretGrants(ctx context.Context) (map[string][]domainsecret.GrantDetails, error)

	// AllSecretConsumers returns all local secret consumers keyed by
	// secret ID.
	AllSecretConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error)

	// AllSecretRemoteConsumers returns all remote secret consumers keyed
	// by secret ID.
	AllSecretRemoteConsumers(ctx context.Context) (map[string][]domainsecret.ConsumerInfo, error)

	// AllRemoteSecrets returns all remote secrets.
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
	// secretID is the logical secret identifier (URI ID), shared across
	// all revisions of the same secret.
	AddSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string, secretID string,
	) (func() error, error)

	// UpdateSecretBackendReference updates the reference
	// to the secret backend for the given secret revision.
	// secretID is the logical secret identifier (URI ID), shared across
	// all revisions of the same secret.
	UpdateSecretBackendReference(
		ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string, secretID string,
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

	// GetSecretBackendNamesByUUID returns a map of backend UUID to backend name for all backends.
	// An empty map will be returned if there are no backends.
	GetSecretBackendNamesByUUID(ctx context.Context) (map[string]string, error)
}
