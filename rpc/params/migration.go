// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/semversion"
)

// MigrationModelHTTPHeader is the key for the HTTP header value
// that is used to specify the model UUID for the model being migrated
// for the uploading of the binaries for that model.
const MigrationModelHTTPHeader = "X-Juju-Migration-Model-UUID"

// InitiateMigrationArgs holds the details required to start one or
// more model migrations.
type InitiateMigrationArgs struct {
	Specs  []MigrationSpec `json:"specs"`
	DryRun bool            `json:"dry-run,omitempty"`
}

// MigrationSpec holds the details required to start the migration of
// a single model.
type MigrationSpec struct {
	ModelTag   string              `json:"model-tag"`
	TargetInfo MigrationTargetInfo `json:"target-info"`
}

// MigrationTargetInfo holds the details required to connect to and
// authenticate with a remote controller for model migration.
type MigrationTargetInfo struct {
	ControllerTag   string   `json:"controller-tag"`
	ControllerAlias string   `json:"controller-alias,omitempty"`
	Addrs           []string `json:"addrs"`
	CACert          string   `json:"ca-cert"`
	AuthTag         string   `json:"auth-tag"`
	Password        string   `json:"password,omitempty"`
	Macaroons       string   `json:"macaroons,omitempty"`
	SkipUserChecks  bool     `json:"skip-user-checks,omitempty"`
	Token           string   `json:"token,omitempty"`
}

// InitiateMigrationResults is used to return the result of one or
// more attempts to start model migrations.
type InitiateMigrationResults struct {
	Results []InitiateMigrationResult `json:"results"`
}

// InitiateMigrationResult is used to return the result of one model
// migration initiation attempt.
type InitiateMigrationResult struct {
	ModelTag    string `json:"model-tag"`
	Error       *Error `json:"error,omitempty"`
	MigrationId string `json:"migration-id"`
}

// SetMigrationPhaseArgs provides a migration phase to the
// migrationmaster.SetPhase API method.
type SetMigrationPhaseArgs struct {
	Phase string `json:"phase"`
}

// SetMigrationStatusMessageArgs provides a migration status message
// to the migrationmaster.SetStatusMessage API method.
type SetMigrationStatusMessageArgs struct {
	Message string `json:"message"`
}

// PrechecksArgs provides the target controller version
// to the migrationmaster.Prechecks API method.
type PrechecksArgs struct {
	TargetControllerVersion semversion.Number `json:"target-controller-version"`
}

// SerializedModel wraps a buffer contain a serialised Juju model. It
// also contains lists of the charms and tools used in the model.
type SerializedModel struct {
	Bytes     []byte                    `json:"bytes"`
	Charms    []string                  `json:"charms"`
	Tools     []SerializedModelTools    `json:"tools"`
	Resources []SerializedModelResource `json:"resources"`
}

// SerializedModelTools holds the version and URI for a given tools
// version.
type SerializedModelTools struct {
	Version string `json:"version"`

	// URI holds the URI were a client can download the tools
	// (e.g. "/tools/1.2.3-xenial-amd64"). It will need to prefixed
	// with the API server scheme, address and model prefix before it
	// can be used.
	URI string `json:"uri"`

	// SHA256 is the sha 256 sum of the tools backed by the URI.
	SHA256 string
}

// SerializedModelResource holds the details for a single resource for
// an application in a serialized model.
type SerializedModelResource struct {
	Application    string    `json:"application"`
	Name           string    `json:"name"`
	Revision       int       `json:"revision"`
	Type           string    `json:"type"`
	Origin         string    `json:"origin"`
	FingerprintHex string    `json:"fingerprint"`
	Size           int64     `json:"size"`
	Timestamp      time.Time `json:"timestamp"`
	Username       string    `json:"username,omitempty"`
}

// SerializedModelV2 is the wire envelope for the new (v8) model-migration
// import/precheck path. Controller-scoped data ride as typed semantic fields;
// only the model-DB content is a serialized YAML payload that flows through the
// transformer chain. The typed fields evolve additively only (the standard
// rpc/params rule) and must never carry source-local integer IDs or
// un-translated source UUID foreign keys.
type SerializedModelV2 struct {
	// PayloadVersion is the model-DB schema version of Payload. It is the single
	// wire version authority for the transformer chain. semversion.Number
	// marshals to the canonical "4.0.6"-style string in both JSON and YAML, so
	// the wire bytes are unchanged versus a string.
	PayloadVersion semversion.Number `json:"payload-version"`

	// Payload is the YAML-encoded concrete generated
	// domain/export/types/vX_Y_Z.ModelExport at PayloadVersion. It is the only
	// field the transformer chain walks. The domain/export.ModelExport wrapper
	// is NOT serialized into this field.
	Payload []byte `json:"payload"`

	// ModelInfo carries the bootstrap identity for the migrating model. It is
	// used by the target to claim the model UUID and create target-local model
	// identity before the rest of the envelope is applied.
	ModelInfo SerializedModelInfo `json:"model-info"`

	// ModelNamespace maps the model UUID to its dqlite namespace name.
	ModelNamespace ModelNamespace `json:"model-namespace"`

	// Users are the controller users referenced by the migrated model, carried
	// by username for recreation or comparison on the target. Authentication
	// material is never carried.
	Users []ModelUser `json:"users,omitempty"`

	// ModelCredential is the model's cloud credential, carried by natural key
	// plus the provider auth attributes needed to create or compare it. Nil when
	// the model has no credential.
	ModelCredential *ModelCloudCredential `json:"model-credential,omitempty"`

	// Permissions carries both object_type='model' and object_type='offer'
	// permission rows for this model, distinguished by ModelPermission.ObjectType.
	Permissions []ModelPermission `json:"permissions,omitempty"`

	// AuthorizedKeys are the SSH key authorisations for the model.
	AuthorizedKeys []ModelAuthorizedKey `json:"authorized-keys,omitempty"`

	// SecretBackend is the secret backend this model uses, by name. Nil when the
	// model uses the controller default.
	SecretBackend *ModelSecretBackend `json:"secret-backend,omitempty"`

	// SecretBackendRefs is the per-revision mapping of secret revisions to
	// backends, by backend name.
	SecretBackendRefs []SecretBackendReference `json:"secret-backend-refs,omitempty"`

	// Leases are the model-scoped application-leadership and singular-controller
	// leases.
	Leases []Lease `json:"leases,omitempty"`

	// LeasePins are the expiry pins for the model's leases, linked by lease
	// natural key.
	LeasePins []LeasePin `json:"lease-pins,omitempty"`

	// LastLogins are the optional per-user last-login timestamps for the model.
	LastLogins []ModelLastLogin `json:"last-logins,omitempty"`

	// CloudImageMetadata are custom image metadata rows to recreate on the
	// target controller.
	CloudImageMetadata []ModelCloudImageMetadata `json:"cloud-image-metadata,omitempty"`

	// ExternalControllers are the third-party controllers (not the source
	// controller) referenced by this model's cross-model relations, with the
	// model UUIDs on each that this model consumes.
	ExternalControllers []ExternalControllerRef `json:"external-controllers,omitempty"`

	// Charms holds the charm URLs whose binaries are transferred separately via
	// the existing /migrate/charms HTTP endpoint.
	Charms []string `json:"charms,omitempty"`

	// Tools holds the agent-tool references transferred via /migrate/tools.
	Tools []SerializedModelTools `json:"tools,omitempty"`

	// Resources holds the resource references transferred via /migrate/resources.
	Resources []SerializedModelResource `json:"resources,omitempty"`
}

// SerializedModelInfo is the bootstrap identity of a migrating model in a
// [SerializedModelV2] envelope. Cloud, region and credential are carried by
// natural key, not by source-local UUID.
type SerializedModelInfo struct {
	// UUID is the model's UUID, preserved across migration.
	UUID string `json:"uuid"`
	// Name is the model name.
	Name string `json:"name"`
	// Qualifier disambiguates Name (the model owner identifier).
	Qualifier string `json:"qualifier"`
	// Type is the model type, e.g. "iaas" or "caas".
	Type string `json:"type"`
	// Cloud is the name of the cloud the model is hosted on.
	Cloud string `json:"cloud"`
	// CloudRegion is the cloud region name; may be empty.
	CloudRegion string `json:"cloud-region,omitempty"`
	// CredentialName is the name of the model's cloud credential; empty when the
	// model has no credential. The full credential rides in
	// SerializedModelV2.ModelCredential.
	CredentialName string `json:"credential-name,omitempty"`
	// CredentialOwner is the username that owns the model's cloud credential.
	CredentialOwner string `json:"credential-owner,omitempty"`
	// Life is the model life value, e.g. "alive".
	Life string `json:"life"`
	// SourceMigrationUUID is the migration UUID from the source side. It is
	// diagnostic only and must be non-empty for a new import claim.
	SourceMigrationUUID string `json:"source-migration-uuid"`
}

// ModelNamespace maps a model UUID to its dqlite namespace name.
type ModelNamespace struct {
	ModelUUID string `json:"model-uuid"`
	Namespace string `json:"namespace"`
}

// ModelUser is the non-authentication profile of a controller user referenced
// by the migrated model, used by the target to recreate a missing user or
// compare an existing one by username.
type ModelUser struct {
	// Name is the username (natural key).
	Name string `json:"name"`
	// DisplayName is the user's display name.
	DisplayName string `json:"display-name,omitempty"`
	// CreatedBy is the username of the user that created this user.
	CreatedBy string `json:"created-by,omitempty"`
	// CreatedAt is when the user was created.
	CreatedAt time.Time `json:"created-at,omitempty"`
	// Removed reports whether the source controller user row was marked removed.
	Removed bool `json:"removed,omitempty"`
	// External reports whether the source controller user row is external.
	External bool `json:"external,omitempty"`
}

// ModelCloudCredential is the model's cloud credential carried by natural key
// (Cloud, Owner, Name) plus the provider auth attributes. The target creates it
// if absent or compares it if present.
type ModelCloudCredential struct {
	// Cloud is the cloud name the credential is for.
	Cloud string `json:"cloud"`
	// Owner is the username that owns the credential.
	Owner string `json:"owner"`
	// Name is the credential name.
	Name string `json:"name"`
	// AuthType is the credential auth type, e.g. "access-key".
	AuthType string `json:"auth-type"`
	// Attributes are the provider auth attributes for the credential.
	Attributes map[string]string `json:"attributes,omitempty"`
	// Revoked reports whether the credential has been revoked.
	Revoked bool `json:"revoked,omitempty"`
	// Invalid reports whether Juju has marked the credential invalid.
	Invalid bool `json:"invalid,omitempty"`
	// InvalidReason describes why the credential was marked invalid.
	InvalidReason string `json:"invalid-reason,omitempty"`
}

// ModelPermission is a single permission grant on the model or on an offer in
// the model. The grantee is carried by username, not user UUID.
type ModelPermission struct {
	// ObjectType is "model" or "offer".
	ObjectType string `json:"object-type"`
	// GrantOn is the model UUID (for ObjectType="model") or the offer UUID (for
	// ObjectType="offer") the access is granted on.
	GrantOn string `json:"grant-on"`
	// SubjectName is the username the access is granted to.
	SubjectName string `json:"subject-name"`
	// Access is the access level, e.g. "read", "admin", "consume".
	Access string `json:"access"`
}

// ModelAuthorizedKey is an SSH public key authorised for the model, carried by
// username and key material rather than the source-local key id.
type ModelAuthorizedKey struct {
	// Username is the owner of the public key.
	Username string `json:"username"`
	// PublicKey is the SSH public key material.
	PublicKey string `json:"public-key"`
}

// ModelSecretBackend identifies the secret backend the model uses, by name.
type ModelSecretBackend struct {
	// Name is the secret backend name (natural key on the target).
	Name string `json:"name"`
	// BackendType is the backend type, e.g. "vault", "kubernetes".
	BackendType string `json:"backend-type,omitempty"`
}

// SecretBackendReference maps a model secret revision to the backend that holds
// it, by backend name. The target rewrites backend names to its own backend
// UUIDs on import.
type SecretBackendReference struct {
	// BackendName is the name of the backend holding the revision.
	BackendName string `json:"backend-name"`
	// SecretRevisionUUID is the model-DB-scoped secret revision UUID.
	SecretRevisionUUID string `json:"secret-revision-uuid"`
	// SecretID is the logical secret identifier shared by all revisions of a
	// secret.
	SecretID string `json:"secret-id"`
}

// Lease is a model-scoped lease (application-leadership or
// singular-controller). It is carried by its natural key (Type + Name) rather
// than the source-local lease UUID so LeasePin can reference it portably.
type Lease struct {
	// Type is "application-leadership" or "singular-controller".
	Type string `json:"type"`
	// Name is the lease name (e.g. the application name for leadership leases).
	Name string `json:"name"`
	// Holder is the entity holding the lease.
	Holder string `json:"holder"`
	// Start is when the lease was acquired.
	Start time.Time `json:"start"`
	// Expiry is when the lease expires.
	Expiry time.Time `json:"expiry"`
}

// LeasePin pins a lease so it cannot expire. It references its lease by the
// lease natural key rather than the source-local lease UUID.
type LeasePin struct {
	// LeaseType is the type of the pinned lease.
	LeaseType string `json:"lease-type"`
	// LeaseName is the name of the pinned lease.
	LeaseName string `json:"lease-name"`
	// EntityID is the entity that pinned the lease.
	EntityID string `json:"entity-id"`
}

// ModelLastLogin is a per-user last-login timestamp for the model, carried by
// username.
type ModelLastLogin struct {
	Username string    `json:"username"`
	Time     time.Time `json:"time"`
}

// ModelCloudImageMetadata carries one custom cloud image metadata row to
// recreate on the target controller. It includes the source creation time for
// migration fidelity without changing the imagemetadatamanager facade shape.
type ModelCloudImageMetadata struct {
	// Stream contains reference to a particular stream, e.g. "released".
	Stream string `json:"stream,omitempty"`
	// Region is the name of cloud region associated with the image.
	Region string `json:"region"`
	// Version is OS version, for e.g. "22.04".
	Version string `json:"version"`
	// Arch is the architecture for this cloud image, for e.g. "amd64".
	Arch string `json:"arch"`
	// VirtType contains the virtualisation type of the cloud image.
	VirtType string `json:"virt-type,omitempty"`
	// RootStorageType contains type of root storage.
	RootStorageType string `json:"root-storage-type,omitempty"`
	// RootStorageSize contains size of root storage in gigabytes (GB).
	RootStorageSize *uint64 `json:"root-storage-size,omitempty"`
	// Source describes where this image is coming from.
	Source string `json:"source"`
	// Priority is an importance factor for image metadata.
	Priority int `json:"priority"`
	// ImageId is an image identifier.
	ImageId string `json:"image-id"`
	// CreatedAt is when the cloud image metadata row was created.
	CreatedAt time.Time `json:"created-at,omitempty"`
}

// ExternalControllerRef carries the connection details for a single third-party
// controller referenced by this model's CMRs, plus the list of model UUIDs on
// that controller that this model consumes (external_model rows).
type ExternalControllerRef struct {
	UUID      string   `json:"uuid"`
	Alias     string   `json:"alias,omitempty"`
	CACert    string   `json:"ca-cert"`
	Addresses []string `json:"addresses,omitempty"`
	// ConsumedModels are the UUIDs of models on this controller consumed by this
	// model's cross-model relations.
	ConsumedModels []string `json:"consumed-models,omitempty"`
}

// ModelArgs wraps a simple model tag.
type ModelArgs struct {
	ModelTag string `json:"model-tag"`
}

// ActivateModelArgs holds args used to
// activate a newly migrated model.
type ActivateModelArgs struct {
	// ModelTag is the model being migrated.
	ModelTag string `json:"model-tag"`

	// ControllerTag is the tag of the source controller.
	ControllerTag string `json:"controller-tag"`
	// ControllerAlias is the name of the source controller.
	ControllerAlias string `json:"controller-alias,omitempty"`
	// SourceAPIAddrs are the api addresses of the source controller.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	// SourceCACert is the CA certificate used to connect to the source controller.
	SourceCACert string `json:"source-ca-cert"`
	// CrossModelUUIDs are the UUIDs of models containing offers to which
	// consumers in the migrated model are related.
	CrossModelUUIDs []string `json:"cross-model-uuids"`
}

// MasterMigrationStatus is used to report the current status of a
// model migration for the migrationmaster. It includes authentication
// details for the remote controller.
type MasterMigrationStatus struct {
	Spec             MigrationSpec `json:"spec"`
	MigrationId      string        `json:"migration-id"`
	Phase            string        `json:"phase"`
	PhaseChangedTime time.Time     `json:"phase-changed-time"`
}

// MigrationModelInfo is used to report basic model information to the
// migrationmaster worker.
type MigrationModelInfo struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	// Qualifier is the model owner identifier used to disambiguate Name.
	Qualifier              string            `json:"qualifier"`
	AgentVersion           semversion.Number `json:"agent-version"`
	ControllerAgentVersion semversion.Number `json:"controller-agent-version"`
	FacadeVersions         map[string][]int  `json:"facade-versions,omitempty"`
	ModelDescription       []byte            `json:"model-description,omitempty"`
}

// MigrationStatus reports the current status of a model migration.
type MigrationStatus struct {
	MigrationId string `json:"migration-id"`
	Attempt     int    `json:"attempt"`
	Phase       string `json:"phase"`

	// TODO(mjs): I'm not convinced these Source fields will get used.
	SourceAPIAddrs []string `json:"source-api-addrs"`
	SourceCACert   string   `json:"source-ca-cert"`

	TargetAPIAddrs []string `json:"target-api-addrs"`
	TargetCACert   string   `json:"target-ca-cert"`
}

// PhaseResults holds the phase of one or more model migrations.
type PhaseResults struct {
	Results []PhaseResult `json:"results"`
}

// PhaseResult holds the phase of a single model migration, or an
// error if the phase could not be determined.
type PhaseResult struct {
	Phase string `json:"phase,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// MinionReport holds the details of whether a migration minion
// succeeded or failed for a specific migration phase.
type MinionReport struct {
	// MigrationId holds the id of the migration the agent is
	// reporting about.
	MigrationId string `json:"migration-id"`

	// Phase holds the phase of the migration the agent is
	// reporting about.
	Phase string `json:"phase"`

	// Success is true if the agent successfully completed its actions
	// for the migration phase, false otherwise.
	Success bool `json:"success"`
}

// MinionReports holds the details of whether a migration minion
// succeeded or failed for a specific migration phase.
type MinionReports struct {
	// MigrationId holds the id of the migration the reports related to.
	MigrationId string `json:"migration-id"`

	// Phase holds the phase of the migration the reports related to.
	Phase string `json:"phase"`

	// SuccessCount holds the number of agents which have successfully
	// completed a given migration phase.
	SuccessCount int `json:"success-count"`

	// UnknownCount holds the number of agents still to report for a
	// given migration phase.
	UnknownCount int `json:"unknown-count"`

	// UnknownSample holds the tags of a limited number of agents
	// that are still to report for a given migration phase (for
	// logging or showing in a user interface).
	UnknownSample []string `json:"unknown-sample"`

	// Failed contains the tags of all agents which have reported a
	// failed to complete a given migration phase.
	Failed []string `json:"failed"`
}

// AdoptResourcesArgs holds the information required to ask the
// provider to update the controller tags for a model's
// resources.
type AdoptResourcesArgs struct {
	// ModelTag identifies the model that owns the resources.
	ModelTag string `json:"model-tag"`

	// SourceControllerVersion indicates the version of the calling
	// controller. This is needed in case the way the resources are
	// tagged has changed between versions - the provider should
	// ensure it looks for the original tags in the correct format for
	// that version.
	SourceControllerVersion semversion.Number `json:"source-controller-version"`
}

// CreateMigrationMacaroonResult holds the reusable login macaroon minted by
// the target controller for the authenticated admin user. The migrationmaster
// worker presents this macaroon when reconnecting to the target, so that a
// cleartext admin password never needs to be persisted in the controller DB.
type CreateMigrationMacaroonResult struct {
	Macaroon *macaroon.Macaroon `json:"macaroon"`
}
