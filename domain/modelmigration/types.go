// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"time"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
)

// MigrationMachineDiscrepancy describes a divergent machine between what Juju
// has and what the cloud has reported. If both the MachineName and the
// CloudInstanceId are both not empty then the discrepancy is on the Juju side
// where we are referencing a instance that doesn't exist in the cloud.
//
// If MachineName is empty then the discrepancy comes from the cloud where a
// instance exists that is not being tracked by Juju.
type MigrationMachineDiscrepancy struct {
	// MachineName is the name given to a machine in the Juju model
	MachineName machine.Name

	// CloudInstanceId is the unique id given to an instance from the cloud.
	CloudInstanceId instance.Id
}

// MigrationMode specifies where the Model is with respect to migration.
type MigrationMode string

const (
	// MigrationModeNone is the default mode for a model and reflects
	// that it isn't involved with a model migration.
	MigrationModeNone = MigrationMode("")

	// MigrationModeExporting reflects a model that is in the process of being
	// exported from one controller to another.
	MigrationModeExporting = MigrationMode("exporting")

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting = MigrationMode("importing")
)

type Migration struct {
	UUID             string
	Phase            migration.Phase
	PhaseChangedTime time.Time
	Target           migration.TargetInfo
}

// ControllerModelInfo aggregates the controller-database facts scoped to a
// single migrating model, in target-portable semantic form. Source-local
// integer IDs and un-translated source UUID foreign keys are never present:
// users are identified by username, SSH keys by their material, clouds, regions
// and credentials by natural key, and secret backends by name.
type ControllerModelInfo struct {
	// ModelInfo is the model's bootstrap identity.
	ModelInfo ModelBootstrapInfo
	// ModelNamespace is the dqlite namespace name for the model's database.
	ModelNamespace string
	// Users are the external users with access to the model or its hosted offers.
	Users []ModelUser
	// ModelCredential is the model's cloud credential, or nil if it has none.
	ModelCredential *ModelCloudCredential
	// Permissions are the model and offer permission grants for the model.
	Permissions []ModelPermission
	// AuthorizedKeys are the SSH keys authorised for the model.
	AuthorizedKeys []ModelAuthorizedKey
	// SecretBackend is the secret backend the model uses, or nil for the default.
	SecretBackend *ModelSecretBackend
	// SecretBackendRefs maps the model's secret revisions to their backends.
	SecretBackendRefs []SecretBackendReference
	// Leases are the model-scoped leases.
	Leases []Lease
	// LeasePins are the expiry pins for the model's leases.
	LeasePins []LeasePin
	// LastLogins are the per-user last-login timestamps for the model.
	LastLogins []ModelLastLogin
	// CloudImageMetadata is custom cloud image metadata that must be recreated
	// on the target controller.
	CloudImageMetadata []CloudImageMetadata
	// ExternalControllers are the third-party controllers referenced by the
	// model's cross-model relations.
	ExternalControllers []ExternalController
}

// ModelBootstrapInfo is the model's bootstrap identity, with cloud, region and
// credential carried by natural key.
type ModelBootstrapInfo struct {
	UUID            string
	Name            string
	Qualifier       string
	Type            string
	Cloud           string
	CloudRegion     string
	CredentialName  string
	CredentialOwner string
	Life            string
}

// ModelUser is the non-authentication profile of an external user with access
// to the model or its hosted offers.
type ModelUser struct {
	Name        string
	DisplayName string
	CreatedBy   string
	CreatedAt   time.Time
}

// ModelCloudCredential is the model's cloud credential, carried by natural key
// (Cloud, Owner, Name) plus the provider auth attributes.
type ModelCloudCredential struct {
	Cloud         string
	Owner         string
	Name          string
	AuthType      string
	Attributes    map[string]string
	Revoked       bool
	Invalid       bool
	InvalidReason string
}

// ModelPermission is a single permission grant on the model or on an offer in
// the model, with the grantee carried by username.
type ModelPermission struct {
	ObjectType  string
	GrantOn     string
	SubjectName string
	Access      string
}

// ModelAuthorizedKey is an SSH public key authorised for the model, carried by
// username and key material.
type ModelAuthorizedKey struct {
	Username  string
	PublicKey string
}

// ModelSecretBackend identifies the secret backend the model uses, by name.
type ModelSecretBackend struct {
	Name        string
	BackendType string
}

// SecretBackendReference maps a model secret revision to its backend, by
// backend name.
type SecretBackendReference struct {
	BackendName        string
	SecretRevisionUUID string
	SecretID           string
}

// Lease is a model-scoped lease, carried by its natural key (Type + Name).
type Lease struct {
	Type   string
	Name   string
	Holder string
	Start  time.Time
	Expiry time.Time
}

// LeasePin pins a lease so it cannot expire, referencing its lease by natural
// key.
type LeasePin struct {
	LeaseType string
	LeaseName string
	EntityID  string
}

// ModelLastLogin is a per-user last-login timestamp for the model, by username.
type ModelLastLogin struct {
	Username string
	Time     time.Time
}

// CloudImageMetadata is one custom cloud image metadata row, carried by
// semantic fields rather than source-local integer IDs.
type CloudImageMetadata struct {
	Stream          string
	Region          string
	Version         string
	Arch            string
	VirtType        string
	RootStorageType string
	RootStorageSize *uint64
	Source          string
	Priority        int
	ImageID         string
	CreatedAt       time.Time
}

// ExternalController carries the connection details for a single third-party
// controller referenced by the model's cross-model relations, plus the model
// UUIDs on that controller that the model consumes.
type ExternalController struct {
	UUID           string
	Alias          string
	CACert         string
	Addresses      []string
	ConsumedModels []string
}
