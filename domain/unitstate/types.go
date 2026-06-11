// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitstate

import (
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/domain/secret"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

const (
	IngressAddressKey = "ingress-address"
	EgressSubnetsKey  = "egress-subnets"
)

// UnitState represents the state of the world according to a unit agent at
// hook commit time.
type UnitState struct {
	// Name is the unit name.
	Name string

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// UniterState is the uniter's state as a YAML string.
	UniterState *string

	// RelationState is key/value pairs for relation attributes.
	RelationState *map[int]string

	// StorageState is a YAML string.
	StorageState *string

	// SecretState is a YAML string.
	SecretState *string
}

// RetrievedUnitState represents a unit state persisted and then retrieved
// from the database.
type RetrievedUnitState struct {
	// Name is the unit name.
	Name string

	// CharmState is key/value pairs for charm attributes.
	CharmState map[string]string

	// UniterState is the uniter's state as a YAML string.
	UniterState string

	// RelationState is key/value pairs for relation attributes.
	RelationState map[int]string

	// StorageState is a YAML string.
	StorageState string

	// SecretState is a YAML string.
	SecretState string
}

// Settings holds relation settings names and values.
type Settings map[string]string

// RelationSettings holds a relation key and local unit and
// app-level settings.
type RelationSettings struct {
	// RelationKey is the Key of the relation.
	RelationKey relation.Key

	// Settings represent the settings of the unit.
	Settings Settings

	// ApplicationSettings represent the settings of the unit.
	ApplicationSettings Settings
}

// CreateSecretArg holds the args for creating a secret.
type CreateSecretArg struct {
	secret.CreateCharmSecretParams

	// URI identifies the secret to create.
	// If empty, the controller generates a URI.
	URI *secrets.URI
}

// UpdateSecretArg holds the args for updating a secret, including
// the URI.
type UpdateSecretArg struct {
	secret.UpdateCharmSecretParams

	// URI identifies the secret to update.
	URI *secrets.URI
}

// GrantSecretArg holds the pre-resolved args for granting access to a
// secret, including all fields needed for the secret_permission upsert.
type GrantSecretArg struct {
	// URI identifies the secret to grant access on.
	URI *secrets.URI

	// SubjectUUID is the resolved UUID of the entity gaining access.
	SubjectUUID string

	// SubjectTypeID is the type of the subject entity.
	SubjectTypeID secret.GrantSubjectType

	// ScopeUUID is the resolved UUID of the access scope entity.
	ScopeUUID string

	// ScopeTypeID is the type of the scope entity.
	ScopeTypeID secret.GrantScopeType

	// RoleID is the role being granted.
	RoleID secret.Role

	// OwnerKind indicates whether the secret is owned by the application
	// or the unit. This drives the leadership requirement.
	OwnerKind secret.CharmSecretOwnerKind
}

// RevokeSecretArg holds the pre-resolved args for revoking access to a
// secret, including the URI and the resolved subject UUID.
type RevokeSecretArg struct {
	// URI identifies the secret to revoke access on.
	URI *secrets.URI

	// SubjectUUID is the resolved UUID of the entity losing access.
	SubjectUUID string

	// SubjectTypeID is the type of the subject entity.
	SubjectTypeID secret.GrantSubjectType

	// OwnerKind indicates whether the secret is owned by the application
	// or the unit. This drives the leadership requirement.
	OwnerKind secret.CharmSecretOwnerKind
}

// DeleteSecretArg holds the args for deleting a secret, including
// the URI.
type DeleteSecretArg struct {
	secret.DeleteSecretParams

	// URI identifies the secret to delete.
	URI *secrets.URI

	// OwnerKind indicates whether the secret is owned by the application
	// or the unit. This drives the leadership requirement: only
	// application-owned deletes need the lease held during the transaction.
	OwnerKind secret.CharmSecretOwnerKind
}

// PreparedStorageAdd holds a storage add request prepared for transactional
// commit-hook handling.
type PreparedStorageAdd struct {
	// StorageName is the storage directive name on the unit.
	StorageName corestorage.Name

	// Storage contains the prepared storage add writes.
	Storage domainstorage.IAASUnitAddStorageArg
}

// CommitHookChangesArg contains data needed to commit a hook change.
type CommitHookChangesArg struct {
	// UnitName is the name of the unit these changes pertain to.
	UnitName unit.Name

	// UpdatedRelationNetworkInfo contain data to update relation network
	// settings this unit.
	UpdatedRelationNetworkInfo map[relation.UUID]Settings

	// RelationUnitSettings settings for the relation unit and application
	// which need to be updated.
	RelationSettings []RelationSettings

	// OpenPorts are GroupedPortRanges with ports to be opened.
	// PortRanges are grouped by relation endpoint name.
	OpenPorts network.GroupedPortRanges

	// ClosePorts are GroupedPortRanges with ports to be closed.
	// PortRanges are grouped by relation endpoint name.
	ClosePorts network.GroupedPortRanges

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// SecretCreates contains charm secrets to create.
	SecretCreates []CreateSecretArg

	// TrackLatestSecrets is a slice of URIs for which the latest revision should
	// be tracked.
	TrackLatestSecrets []string

	// SecretUpdates contains charm secrets to update.
	SecretUpdates []UpdateSecretArg

	// SecretGrants contains pre-resolved charm secret grant requests.
	SecretGrants []GrantSecretArg

	// SecretRevokes contains pre-resolved charm secret revoke requests.
	SecretRevokes []RevokeSecretArg

	// SecretDeletes contains charm secrets to delete.
	SecretDeletes []DeleteSecretArg

	// AddStorage contains prepared unit storage adds to apply in the commit
	// hook transaction.
	AddStorage []PreparedStorageAdd
}

// ValidateAndHasChanges validates that:
// - that a unit name and uuid are provided and valid
// - relation settings have a valid relation uuid
// - port ranges are valid if provided
// - secret changes requiring a URI have them
// Returns true if there are changes.
func (c CommitHookChangesArg) ValidateAndHasChanges() (bool, error) {
	errs := []error{}
	if err := c.UnitName.Validate(); err != nil {
		errs = append(errs, err)
	}
	var hasChanges bool
	if c.CharmState != nil {
		hasChanges = true
	}
	if len(c.UpdatedRelationNetworkInfo) > 0 {
		hasChanges = true
	}
	for _, settings := range c.RelationSettings {
		hasChanges = true
		if err := settings.RelationKey.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, portRange := range c.OpenPorts {
		hasChanges = true
		for _, port := range portRange {
			if err := port.Validate(); err != nil {
				errs = append(errs, errors.Errorf("open port is invalid: %w", err))
			}
		}
	}
	for _, portRange := range c.ClosePorts {
		hasChanges = true
		for _, port := range portRange {
			if err := port.Validate(); err != nil {
				errs = append(errs, errors.Errorf("close port is invalid: %w", err))
			}
		}
	}
	if err := c.verifyNoPortRangeConflicts(); err != nil {
		errs = append(errs, errors.Errorf("cannot update unit ports with conflict(s): %w", err))
	}
	if len(c.SecretCreates) > 0 {
		// If the URI is empty in a CreateSecretArg, it will be created.
		hasChanges = true
	}
	if len(c.TrackLatestSecrets) > 0 {
		hasChanges = true
	}
	for _, secret := range c.SecretUpdates {
		hasChanges = true
		if secret.URI == nil {
			errs = append(errs, errors.New("secret uri is required for update"))
			break
		}
	}
	for _, secret := range c.SecretGrants {
		hasChanges = true
		if secret.URI == nil {
			errs = append(errs, errors.New("secret uri is required for grant"))
			break
		}
		if secret.SubjectUUID == "" {
			errs = append(errs, errors.New("subject uuid is required for grant"))
			break
		}
		if secret.ScopeUUID == "" {
			errs = append(errs, errors.New("scope uuid is required for grant"))
			break
		}
	}
	for _, secret := range c.SecretRevokes {
		hasChanges = true
		if secret.URI == nil {
			errs = append(errs, errors.New("secret uri is required for revoke"))
			break
		}
		if secret.SubjectUUID == "" {
			errs = append(errs, errors.New("subject uuid is required for revoke"))
			break
		}
	}
	for _, secret := range c.SecretDeletes {
		hasChanges = true
		if secret.URI == nil {
			errs = append(errs, errors.New("secret uri is required for delete"))
			break
		}
	}
	if len(c.AddStorage) > 0 {
		hasChanges = true
	}
	return hasChanges, errors.Join(errs...)
}

// RequiresLeadership returns true if the commit requires the unit to
// hold the application lease during the transaction. This is needed for
// application-level relation settings and for application-owned secret
// operations. Unit-owned secret operations are allowed without
// leadership since the unit is the sole authority over its own secrets.
func (c CommitHookChangesArg) RequiresLeadership() bool {
	for _, settings := range c.RelationSettings {
		if len(settings.ApplicationSettings) > 0 {
			return true
		}
	}
	for _, s := range c.SecretCreates {
		if s.CharmOwner.Kind == secret.ApplicationCharmSecretOwner {
			return true
		}
	}
	for _, s := range c.SecretDeletes {
		if s.OwnerKind == secret.ApplicationCharmSecretOwner {
			return true
		}
	}
	for _, s := range c.SecretGrants {
		if s.OwnerKind == secret.ApplicationCharmSecretOwner {
			return true
		}
	}
	for _, s := range c.SecretRevokes {
		if s.OwnerKind == secret.ApplicationCharmSecretOwner {
			return true
		}
	}
	// Updates currently go through their own service calls
	// (outside the txn) which handle leadership internally. Once they
	// move into the transaction, they will need similar owner-awareness
	// here.
	if len(c.SecretUpdates) > 0 {
		return true
	}
	// TrackLatestSecrets is a per-unit consumer operation; leadership is not
	// required.
	return false
}

// verifyNoPortRangeConflicts verifies the provided port ranges do not conflict
// with each other.
//
// A conflict occurs when two (or more) port ranges across all endpoints overlap,
// but are not equal.
func (c CommitHookChangesArg) verifyNoPortRangeConflicts() error {
	if len(c.OpenPorts)+len(c.ClosePorts) == 0 {
		return nil
	}
	allInputPortRanges := append(c.OpenPorts.UniquePortRanges(), c.ClosePorts.UniquePortRanges()...)
	var conflicts []error
	for _, portRange := range allInputPortRanges {
		for _, otherPortRange := range allInputPortRanges {
			if portRange != otherPortRange && portRange.ConflictsWith(otherPortRange) {
				conflicts = append(conflicts, errors.Errorf("[%s, %s]", portRange, otherPortRange))
			}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	return errors.Errorf("%s: %w", errors.Join(conflicts...), porterrors.PortRangeConflict)
}
