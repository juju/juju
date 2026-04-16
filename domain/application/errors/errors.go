// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"math"
	"strconv"

	"github.com/juju/juju/internal/errors"
)

// CharmStorageLocationProhibited describes an error that occurs when a charms
// storage requests a location that is prohibited with a Juju controller.
type CharmStorageLocationProhibited struct {
	// CharmStorageLocation is the location requested by the charm for mounting
	// the storage.
	CharmStorageLocation string

	// CharmStorageName is the name of the storage as referenced by the charm.
	CharmStorageName string

	// ProhibitedLocation is the prohibited location that has been violated by
	// the charm.
	ProhibitedLocation string
}

// UnitStorageMinViolation describes an error that occurs when a unit's charm
// required minimum storage count would have been violated. An example of this
// is if a charm requires 3 storage instances for "data" but an operation on a
// unit would have resulted in less than the minimum.
type UnitStorageMinViolation struct {
	// CharmStorageName is the name of the storage as referenced by the charm.
	CharmStorageName string

	// RequiredMinimum is the minimum number of storage instances required by
	// the charm for this storage.
	RequiredMinimum int

	// UnitUUID is the uuid of the unit for which the violation has occurred.
	UnitUUID string
}

const (
	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ApplicationAlreadyExists describes an error that occurs when the
	// application being created already exists.
	ApplicationAlreadyExists = errors.ConstError("application already exists")

	// ApplicationNotAlive describes an error that occurs when trying to update
	// an application that is not alive.
	ApplicationNotAlive = errors.ConstError("application is not alive")

	// ApplicationIsDead describes an error that occurs when trying to access
	// an application that is dead.
	ApplicationIsDead = errors.ConstError("application is dead")

	// ApplicationHasUnits describes an error that occurs when an operation on
	// an application fails because the application has units associated.
	ApplicationHasUnits = errors.ConstError("application has units")

	// ApplicationHasRelations describes an error that occurs when an operation
	// on an application fails because the application has relations associated.
	ApplicationHasRelations = errors.ConstError("application has relations")

	// ApplicationNotSubordinate describes an error that occurs when a
	// subordinate application is expected but a prinicpal application is found.
	ApplicationNotSubordinate = errors.ConstError("application not subordinate")

	// ScalingStateInconsistent is returned by SetScalingState when the scaling
	// state is inconsistent with the application scale.
	ScalingStateInconsistent = errors.ConstError("scaling state is inconsistent")

	// ScaleChangeInvalid is returned when an attempt is made to set an invalid
	// application scale value.
	ScaleChangeInvalid = errors.ConstError("scale change invalid")

	// MissingStorageDirective describes an error that occurs when expected
	// storage directives are missing.
	MissingStorageDirective = errors.ConstError("no storage directive specified")

	// ApplicationNameNotValid describes an error when the application is
	// not valid.
	ApplicationNameNotValid = errors.ConstError("application name not valid")

	// ApplicationUUIDNotValid describes an error when the application UUID is
	// not valid.
	ApplicationUUIDNotValid = errors.ConstError("application UUID not valid")

	// UnitNotFound describes an error that occurs when the unit being operated
	// on does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitStatusNotFound describes an error that occurs when the unit being
	// operated on does not have a status.
	UnitStatusNotFound = errors.ConstError("unit status not found")

	// UnitAlreadyExists describes an error that occurs when the
	// unit being created already exists.
	UnitAlreadyExists = errors.ConstError("unit already exists")

	// UnitNotAssigned describes an error that occurs when the unit being
	// operated on is not assigned.
	UnitNotAssigned = errors.ConstError("unit not assigned")

	// UnitHasSubordinates describes an error that occurs when trying to set a
	// unit's life to Dead but it still has subordinates.
	UnitHasSubordinates = errors.ConstError("unit has subordinates")

	// UnitHasStorageAttachments describes an error that occurs when trying to
	// set a unit's life to Dead but it still has storage attachments.
	UnitHasStorageAttachments = errors.ConstError("unit has storage attachments")

	// UnitIsAlive describes an error that occurs when trying to remove a unit
	// that is still alive.
	UnitIsAlive = errors.ConstError("unit is alive")

	// UnitNotAlive describes an error that occurs when trying to update
	// a unit that is not alive.
	UnitNotAlive = errors.ConstError("unit is not alive")

	// UnitIsDead describes an error that occurs when trying to access
	// an application that is dead.
	UnitIsDead = errors.ConstError("unit is dead")

	// UnitUUIDNotValid describes an error when the unit UUID is
	// not valid.
	UnitUUIDNotValid = errors.ConstError("unit UUID not valid")

	// UnitAlreadyHasSubordinate describes an error that occurs when trying to
	// add a subordinate to a unit but one already exists.
	UnitAlreadyHasSubordinate = errors.ConstError("unit already has subordinate")

	// InvalidApplicationState describes an error where the application state is
	// invalid. There are missing required fields.
	InvalidApplicationState = errors.ConstError("invalid application state")

	// CharmNotValid describes an error that occurs when the charm is not valid.
	CharmNotValid = errors.ConstError("charm not valid")

	// CharmOriginNotValid describes an error that occurs when the charm origin
	// is not valid.
	CharmOriginNotValid = errors.ConstError("charm origin not valid")

	// CharmNameNotValid describes an error that occurs when attempting to get
	// a charm using an invalid name.
	CharmNameNotValid = errors.ConstError("charm name not valid")

	// CharmSourceNotValid describes an error that occurs when attempting to get
	// a charm using an invalid charm source.
	CharmSourceNotValid = errors.ConstError("charm source not valid")

	// CharmNotFound describes an error that occurs when a charm cannot be
	// found.
	CharmNotFound = errors.ConstError("charm not found")

	// LXDProfileNotFound describes an error that occurs when an LXD profile
	// cannot be found.
	LXDProfileNotFound = errors.ConstError("LXD profile not found")

	// CharmAlreadyExists describes an error that occurs when a charm already
	// exists for the given natural key.
	CharmAlreadyExists = errors.ConstError("charm already exists")

	// CharmRevisionNotValid describes an error that occurs when attempting to
	// get a charm using an invalid revision.
	CharmRevisionNotValid = errors.ConstError("charm revision not valid")

	// CharmMetadataNotValid describes an error that occurs when the charm
	// metadata is not valid.
	CharmMetadataNotValid = errors.ConstError("charm metadata not valid")

	// CharmManifestNotFound describes an error that occurs when the charm
	// manifest is not found.
	CharmManifestNotFound = errors.ConstError("charm manifest not found")

	// CharmManifestNotValid describes an error that occurs when the charm
	// manifest is not valid.
	CharmManifestNotValid = errors.ConstError("charm manifest not valid")

	// CharmBaseNameNotValid describes an error that occurs when the charm base
	// name is not valid.
	CharmBaseNameNotValid = errors.ConstError("charm base name not valid")

	// CharmBaseNameNotSupported describes an error that occurs when the charm
	// base name is not supported.
	CharmBaseNameNotSupported = errors.ConstError("charm base name not supported")

	// CharmRelationNotFound indicates that a required relation for the charm could not be found.
	CharmRelationNotFound = errors.ConstError("charm relation not found")

	// CharmRelationKeyConflict describes an error that occurs when the charm
	// has multiple relations with the same name
	CharmRelationNameConflict = errors.ConstError("charm relation name conflict")

	// CharmRelationReservedNameMisuse describes an error that occurs when the
	// charm relation name is a reserved name which it is not allowed to use.
	CharmRelationReservedNameMisuse = errors.ConstError("charm relation reserved name misuse")

	// CharmRelationRoleNotValid describes an error that occurs when the charm
	// relation roles is not valid. Either it is an unknown role, or it has the
	// wrong value.
	CharmRelationRoleNotValid = errors.ConstError("charm relation role not valid")

	// MultipleCharmHashes describes and error that occurs when a charm has
	// multiple hash values. At the moment, we only support sha256 hash format,
	// so if another is found, an error is returned.
	MultipleCharmHashes = errors.ConstError("multiple charm hashes found")

	// CharmAlreadyAvailable describes an error that occurs when a charm is
	// already been made available. There is no need to download it again.
	CharmAlreadyAvailable = errors.ConstError("charm already available")

	// UnableToResolveCharm describes an error that occurs when a charm cannot
	// be resolved.
	UnableToResolveCharm = errors.ConstError("unable to resolve charm")

	// CharmAlreadyResolved describes an error that occurs when a charm is
	// already resolved. This means the charm for the hash already exists and
	// can just be used.
	CharmAlreadyResolved = errors.ConstError("charm already resolved")

	// CharmNotResolved describes an error that occurs when a charm is not
	// resolved. This means the charm for the hash does not exist and needs to
	// be downloaded.
	CharmNotResolved = errors.ConstError("charm not resolved")

	// CharmHashMismatch describes an error that occurs when the hash of the
	// downloaded charm does not match the expected hash.
	CharmHashMismatch = errors.ConstError("charm hash mismatch")

	// CharmDownloadInfoNotFound describes an error that occurs when the charm
	// download info is not found.
	CharmDownloadInfoNotFound = errors.ConstError("charm download info not found")

	// CharmDownloadURLNotValid describes an error that occurs when the charm
	// download URL is not valid.
	CharmDownloadURLNotValid = errors.ConstError("charm download URL not valid")

	// CharmHashNotFound describes an error that occurs when the charm hash is
	// not found.
	CharmHashNotFound = errors.ConstError("charm hash not found")

	// CharmProvenanceNotValid describes an error that occurs when the
	// charm download provenance is not valid.
	CharmProvenanceNotValid = errors.ConstError("charm provenance not valid")

	// InvalidResourceArgs indicates the provided resource arguments are not
	// valid, for instance when we try to override non-existing resources in
	// the charm.
	InvalidResourceArgs = errors.ConstError("invalid resource args")

	// CharmSHA256PrefixMismatch describes an error that occurs when the
	// SHA256 prefix of the charm does not match the expected prefix.
	CharmSHA256PrefixMismatch = errors.ConstError("charm SHA256 prefix mismatch")

	// NonLocalCharmImporting describes an error that occurs when the charm is
	// still being imported.
	NonLocalCharmImporting = errors.ConstError("non-local charms may only be uploaded during model migration import")

	// CharmAlreadyExistsWithDifferentSize describes an error that occurs when
	// a charm already exists with a different size. This might not actually be
	// a charm, but chances are that it is.
	CharmAlreadyExistsWithDifferentSize = errors.ConstError("charm already exists with different size")

	// InvalidApplicationConfig describes an error that occurs when the application
	// config is not valid.
	InvalidApplicationConfig = errors.ConstError("invalid application config")

	// ApplicationHasDifferentCharm describes an error that occurs when the
	// application has a different charm.
	ApplicationHasDifferentCharm = errors.ConstError("application has different charm")

	// InvalidApplicationConstraints describes an error that occurs when the
	// application constraints are not valid. This happens when if the
	// provided space constraints do not exist or the container type is not
	// supported.
	InvalidApplicationConstraints = errors.ConstError("invalid application constraints")

	// InvalidUnitConstraints describes an error that occurs when the
	// application constraints are not valid. This happens when if the
	// provided space constraints do not exist or the container type is not
	// supported.
	InvalidUnitConstraints = errors.ConstError("invalid unit constraints")

	// InvalidSecretConfig describes an error that occurs when the secret
	// config is not valid.
	InvalidSecretConfig = errors.ConstError("invalid secret config")

	// SpaceNotFound is returned when the specified space cannot be found.
	SpaceNotFound = errors.ConstError("space not found")

	// StorageInstanceAlreadyAttachedToUnit is returned when an attach operation
	// targets a Unit that already has an attachment for the Storage Instance.
	StorageInstanceAlreadyAttachedToUnit = errors.ConstError(
		"storage instance already attached to unit",
	)

	// StorageInstanceUnexpectedAttachments is returned when a storage instance
	// has attachments that do not match the expected set.
	StorageInstanceUnexpectedAttachments = errors.ConstError(
		"storage instance has unexpected attachments",
	)

	// StorageInstanceAttachSharedAccessNotSupported is returned when attempting
	// to attach a storage instance that already has attachments to other units,
	// but the unit's charm storage definition does not support shared access.
	StorageInstanceAttachSharedAccessNotSupported = errors.ConstError(
		"storage instance attach shared access not supported",
	)

	// EndpointNotFound descries an error that occurs when the endpoint being
	// operated on does not exist.
	EndpointNotFound = errors.ConstError("endpoint not found")

	// MachineNotFound describes an error that occurs when the machine being
	// operated on does not exist.
	MachineNotFound = errors.ConstError("machine not found")

	// UnitMachineNotAssigned describes an error that occurs when a unit is not
	// assigned to a machine.
	UnitMachineNotAssigned = errors.ConstError("unit machine not assigned")

	// NetNodeNotFound describes an error that occurs when the net node being
	// operated on does not exist.
	NetNodeNotFound = errors.ConstError("net node not found")

	// UnitsUpgrading describes an error where units are currently in the process
	// of upgrading. This is relevant for model migration.
	UnitsUpgrading = errors.ConstError("units upgrading")

	// UnitAttachmentCountExceedsLimit describes an error that occurs where a
	// unit's Storage Instance attachment count exceeds a pre-established max
	// limit.
	UnitAttachmentCountExceedsLimit = errors.ConstError(
		"unit attachment count exceeds limit",
	)

	// UnitCharmChanged describes an error that occure when a Unit's charm
	// has changed and no longer mataches a pre-existing assumption about what
	// Charm the Unit was running.
	UnitCharmChanged = errors.ConstError("unit charm has changed")

	// UnitMachineChanged describes an error that occurs when a Unit's machine
	// has changed and no longer matches a pre-existing assumption about the
	// Unit's Machine.
	UnitMachineChanged = errors.ConstError("unit machine has changed")
)

const (
	// StorageNameNotSupported describes an error that occurs when
	// a storage name is not supported by the charm.
	StorageNameNotSupported = errors.ConstError("storage name not supported")

	// StorageInstanceKindNotValidForCharmStorageDefinition describes an error
	// that occurs when a storage instance's kind is not compatible with the
	// kind required by the charm's storage definition.
	StorageInstanceKindNotValidForCharmStorageDefinition = errors.ConstError(
		"storage instance kind not valid for charm storage definition",
	)

	// StorageInstanceCharmNameMismatch describes an error that occurs when a
	// storage instance is associated with a different charm name than the unit.
	StorageInstanceCharmNameMismatch = errors.ConstError(
		"storage instance charm name mismatch",
	)

	// StorageInstanceSizeNotValidForCharmStorageDefinition describes an error
	// that occurs when a storage instance's size does not meet the minimum
	// required by the charm's storage definition.
	StorageInstanceSizeNotValidForCharmStorageDefinition = errors.ConstError(
		"storage instance size not valid for charm storage definition",
	)

	// StorageInstanceAttachMachineOwnerMismatch describes an error that occurs
	// when a storage instance's owning machine does not match the unit's
	// machine.
	StorageInstanceAttachMachineOwnerMismatch = errors.ConstError(
		"storage instance owning machine does not match unit machine",
	)
)

// StorageCountLimitExceeded describes an error that occurs when an operation
// to change the storage count would exceed the supported limits. This error is
// a struct so that the context can be supplied upwards to the caller.
type StorageCountLimitExceeded struct {
	// Maximum when not nill defines the maximum number of storage instances
	// that can be realised.
	Maximum *int

	// Minimum defines the minimum number of storage instances that MUST exist.
	Minimum int

	// Requested is the number of storage instances that were requested by the
	// caller.
	Requested int

	// StorageName is the name of the storage whose count was being changed in
	// the operation.
	StorageName string
}

// Error returns a description on the prohibited location that has been
// violated by a charms storage. This func implements the [error] interface.
func (s CharmStorageLocationProhibited) Error() string {
	return fmt.Sprintf(
		"charm storage %q requested location %q which violates prohibited location %q",
		s.CharmStorageName, s.CharmStorageLocation, s.ProhibitedLocation,
	)
}

// Error returns a description on the storage count limit that has been
// violated. This func implements the [error] interface.
func (s StorageCountLimitExceeded) Error() string {
	if s.Maximum != nil && s.Requested > *s.Maximum {
		return fmt.Sprintf(
			"storage %q cannot exceed %d storage instances",
			s.StorageName, *s.Maximum,
		)
	} else if s.Requested < s.Minimum {
		return fmt.Sprintf(
			"storage %q cannot have less than %d storage instances",
			s.StorageName, s.Minimum,
		)
	}

	// We should never get here but if we do we can at least try to provide a
	// sensible error message within the limits we have.
	maxStr := strconv.Itoa(math.MaxUint32)
	if s.Maximum != nil {
		maxStr = strconv.Itoa(*s.Maximum)
	}
	return fmt.Sprintf(
		"storage %q count must be between %d and %s, requested was %d",
		s.StorageName,
		s.Minimum,
		maxStr,
		s.Requested,
	)
}

// Error returns a string representation of the [UnitStorageMinViolation] error
// providing context for the violation. This func implements the [error]
// interface.
func (u UnitStorageMinViolation) Error() string {
	return fmt.Sprintf(
		"unit %q storage %q minimum requirement %d violated",
		u.UnitUUID, u.CharmStorageName, u.RequiredMinimum,
	)
}

// CharmStorageDefinitionRemoved describes an error that occurs when a charm's
// storage definition is removed during a refresh for a named storage definition.
// Example of this would be the previous charm's storage `foo` being removed
// in the new charm.
type CharmStorageDefinitionRemoved struct {
	// StorageName is the name of the storage which was removed in the new charm,
	// but exists in the old charm.
	StorageName string
}

// Error returns a string representation of the [CharmStorageDefinitionRemoved] error
// providing context of the storage name for the violation.
// This func implements the [error] interface.
func (s CharmStorageDefinitionRemoved) Error() string {
	return fmt.Sprintf(
		"storage definition %q removed", s.StorageName,
	)
}

// CharmStorageTypeChanged describes an error that occurs when a charm's
// storage type changes during a refresh for a named storage definition.
// Example of this would be the previous charm's storage `foo` changing from
// filesystem to block storage.
type CharmStorageTypeChanged struct {
	// StorageName is the name of the storage whose type was being changed in
	// the operation.
	StorageName string
	// OldType is the existing storage type.
	OldType string
	// NewType is the new storage type that is different from the old type.
	NewType string
}

// Error returns a string representation of the [CharmStorageTypeChanged] error
// providing context of the storage name, old type and new type for the violation.
// This func implements the [error] interface.
func (s CharmStorageTypeChanged) Error() string {
	return fmt.Sprintf(
		"storage definition %q type changed from %q to %q",
		s.StorageName, s.OldType, s.NewType,
	)
}

// CharmStorageDefinitionMinSizeViolation describes an error that occurs when a charm's
// storage minimum size increases during a refresh for a named storage definition.
type CharmStorageDefinitionMinSizeViolation struct {
	// ExistingMin defines the minimum storage size required by the existing charm.
	ExistingMin uint64

	// NewMin defines the minimum storage size required by the new charm.
	NewMin uint64

	// StorageName is the name of the storage whose
	// minimum size check fails.
	StorageName string
}

// Error returns a string representation of the [CharmStorageDefinitionMinSizeViolation] error
// providing context for the violation. This func implements the [error]
// interface.
func (s CharmStorageDefinitionMinSizeViolation) Error() string {
	return fmt.Sprintf(
		"storage definition %q new minimum size %d MiB exceeds existing minimum size %d MiB",
		s.StorageName, s.NewMin, s.ExistingMin,
	)
}

// CharmStorageDefinitionMinCountViolation describes an error that occurs when a charm's
// storage minimum count increases during a refresh for a named storage definition.
type CharmStorageDefinitionMinCountViolation struct {
	// ExistingMin defines the minimum storage count required by the existing charm.
	ExistingMin int

	// NewMin defines the minimum storage count required by the new charm.
	NewMin int

	// StorageName is the name of the storage whose
	// minimum count check fails.
	StorageName string
}

// Error returns a string representation of the [CharmStorageDefinitionMinCountViolation] error
// providing context for the violation. This func implements the [error]
// interface.
func (s CharmStorageDefinitionMinCountViolation) Error() string {
	return fmt.Sprintf(
		"storage definition %q new minimum count %d exceeds existing minimum count %d",
		s.StorageName, s.NewMin, s.ExistingMin,
	)
}

// CharmStorageDefinitionMaxCountViolation describes an error that occurs when a charm's
// storage maximum count decreases during a refresh for a named storage definition.
type CharmStorageDefinitionMaxCountViolation struct {
	// ExistingMax defines the maximum storage count required by the existing charm.
	ExistingMax int

	// NewMax defines the maximum storage count required by the new charm.
	NewMax int

	// StorageName is the name of the storage whose
	// maximum count check fails.
	StorageName string
}

// Error returns a string representation of the [CharmStorageDefinitionMaxCountViolation] error
// providing context for the violation. This func implements the [error]
// interface.
func (s CharmStorageDefinitionMaxCountViolation) Error() string {
	if s.ExistingMax < 0 {
		return fmt.Sprintf(
			"storage definition %q new maximum count %d is less than existing maximum count (unbounded)",
			s.StorageName, s.NewMax,
		)
	}
	return fmt.Sprintf(
		"storage definition %q new maximum count %d is less than existing maximum count %d",
		s.StorageName, s.NewMax, s.ExistingMax,
	)
}

// CharmStorageDefinitionSingleToMultipleViolation describes an error that occurs
// when a charm's singleton storage definition with a fixed location changes to
// a multiple storage definition during refresh.
type CharmStorageDefinitionSingleToMultipleViolation struct {
	// ExistingMax defines the existing maximum storage count.
	ExistingMax int

	// NewMax defines the new maximum storage count.
	NewMax int

	// StorageName is the storage whose single-to-multiple check fails.
	StorageName string
}

// Error returns a string representation of the
// [CharmStorageDefinitionSingleToMultipleViolation] error.
func (s CharmStorageDefinitionSingleToMultipleViolation) Error() string {
	return fmt.Sprintf(
		"storage definition %q cannot change from single to multiple when location is set (maximum count %d -> %d)",
		s.StorageName, s.ExistingMax, s.NewMax,
	)
}

// CharmStorageDefinitionSharedChanged describes an error that occurs when a
// charm storage definition shared value changes during refresh.
type CharmStorageDefinitionSharedChanged struct {
	StorageName   string
	ExistingValue bool
	NewValue      bool
}

func (s CharmStorageDefinitionSharedChanged) Error() string {
	return fmt.Sprintf(
		"storage definition %q shared changed from %v to %v",
		s.StorageName, s.ExistingValue, s.NewValue,
	)
}

// CharmStorageDefinitionReadOnlyChanged describes an error that occurs when a
// charm storage definition read-only value changes during refresh.
type CharmStorageDefinitionReadOnlyChanged struct {
	StorageName   string
	ExistingValue bool
	NewValue      bool
}

func (s CharmStorageDefinitionReadOnlyChanged) Error() string {
	return fmt.Sprintf(
		"storage definition %q read-only changed from %v to %v",
		s.StorageName, s.ExistingValue, s.NewValue,
	)
}

// CharmStorageDefinitionLocationChanged describes an error that occurs when a
// charm storage definition location changes during refresh.
type CharmStorageDefinitionLocationChanged struct {
	StorageName      string
	ExistingLocation string
	NewLocation      string
}

func (s CharmStorageDefinitionLocationChanged) Error() string {
	return fmt.Sprintf(
		"storage definition %q location changed from %q to %q",
		s.StorageName, s.ExistingLocation, s.NewLocation,
	)
}
