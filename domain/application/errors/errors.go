// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

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

	// ApplicationHasUnits describes an error that occurs when the application
	// being deleted still has associated units.
	ApplicationHasUnits = errors.ConstError("application has units")

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

	// ApplicationIDNotValid describes an error when the application ID is
	// not valid.
	ApplicationIDNotValid = errors.ConstError("application ID not valid")

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

	// EndpointNotFound descries an error that occurs when the endpoint being
	// operated on does not exist.
	EndpointNotFound = errors.ConstError("endpoint not found")

	// MachineNotFound describes an error that occurs when the machine being
	// operated on does not exist.
	MachineNotFound = errors.ConstError("machine not found")
)

const (
	// StorageAlreadyAttached describes an error that occurs when
	// a storage attachment already exists.
	StorageAlreadyAttached = errors.ConstError("storage already attached")

	// FilesystemAlreadyAttached describes an error that occurs when
	// a filesystem attachment already exists.
	FilesystemAlreadyAttached = errors.ConstError("filesystem already attached")

	// VolumeAlreadyAttached describes an error that occurs when
	// a volume attachment already exists.
	VolumeAlreadyAttached = errors.ConstError("volume already attached")

	// StorageNotAlive describes an error that occurs when
	// a storage is not alive.
	StorageNotAlive = errors.ConstError("storage not alive")

	// StorageNameNotSupported describes an error that occurs when
	// a storage name is not supported by the charm.
	StorageNameNotSupported = errors.ConstError("storage name not supported")

	// InvalidStorageCount describes an error that occurs when
	// a storage attachment would violate charm expectations for cardinality.
	InvalidStorageCount = errors.ConstError("invalid storage count")

	// StorageNotDetachable describes an error that occurs when
	// a storage does not support being detached.
	StorageNotDetachable = errors.ConstError("storage cannot be detached")

	// InvalidStorageMountPoint describes an error that occurs when
	// a storage attachment's location cannot be mounted on the node.
	InvalidStorageMountPoint = errors.ConstError("invalid storage mount point")
)
