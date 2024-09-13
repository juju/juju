// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// ApplicationNotFound describes an error that occurs when the application
	// being operated on does not exist.
	ApplicationNotFound = errors.ConstError("application not found")

	// ApplicationAlreadyExists describes an error that occurs when the
	// application being created already exists.
	ApplicationAlreadyExists = errors.ConstError("application already exists")

	// ApplicationHasUnits describes an error that occurs when the application
	// being deleted still has associated units.
	ApplicationHasUnits = errors.ConstError("application has units")

	// ScalingStateInconsistent is returned by SetScalingState when the scaling state
	// is inconsistent with the application scale.
	ScalingStateInconsistent = errors.ConstError("scaling state is inconsistent")

	// ScaleChangeInvalid is returned when an attempt is made to set an invalid application scale value.
	ScaleChangeInvalid = errors.ConstError("scale change invalid")

	// MissingStorageDirective describes an error that occurs when expected
	// storage directives are missing.
	MissingStorageDirective = errors.ConstError("no storage directive specified")

	// ApplicationNameNotValid describes an error when the application is
	// not valid.
	ApplicationNameNotValid = errors.ConstError("application name not valid")

	// UnitNotFound describes an error that occurs when the unit being operated on
	// does not exist.
	UnitNotFound = errors.ConstError("unit not found")

	// UnitAlreadyExists describes an error that occurs when the
	// unit being created already exists.
	UnitAlreadyExists = errors.ConstError("unit already exists")

	// UnitNotAssigned describes an error that occurs when the unit being operated on
	// is not assigned.
	UnitNotAssigned = errors.ConstError("unit not assigned")

	// UnitAlreadyExists describes an error that occurs when the
	// unit being created already exists.
	UnitAlreadyExists = errors.ConstError("unit already exists")

	// UnitHasSubordinates describes an error that occurs when trying to set a unit's life
	// to Dead but it still has subordinates.
	UnitHasSubordinates = errors.ConstError("unit has subordinates")

	// UnitHasStorageAttachments describes an error that occurs when trying to set a unit's life
	// to Dead but it still has storage attachments.
	UnitHasStorageAttachments = errors.ConstError("unit has storage attachments")

	// UnitIsAlive describes an error that occurs when trying to remove a unit that is still alive.
	UnitIsAlive = errors.ConstError("unit is alive")
	// InvalidApplicationState describes an error where the application state is invalid.
	// There are missing required fields.
	InvalidApplicationState = errors.ConstError("invalid application state")

	// CharmNotValid describes an error that occurs when the charm is not valid.
	CharmNotValid = errors.ConstError("charm not valid")

	// CharmOriginNotValid describes an error that occurs when the charm origin is not valid.
	CharmOriginNotValid = errors.ConstError("charm origin not valid")

	// CharmNameNotValid describes an error that occurs when attempting to get
	// a charm using an invalid name.
	CharmNameNotValid = errors.ConstError("charm name not valid")

	// CharmSourceNotValid describes an error that occurs when attempting to get
	// a charm using an invalid charm source.
	CharmSourceNotValid = errors.ConstError("charm source not valid")

	// CharmNotFound describes an error that occurs when a charm cannot be found.
	CharmNotFound = errors.ConstError("charm not found")

	// LXDProfileNotFound describes an error that occurs when an LXD profile
	// cannot be found.
	LXDProfileNotFound = errors.ConstError("LXD profile not found")

	// CharmAlreadyExists describes an error that occurs when a charm already
	// exists for the given natural key.
	CharmAlreadyExists = errors.ConstError("charm already exists")

	// CharmRevisionNotValid describes an error that occurs when attempting to get
	// a charm using an invalid revision.
	CharmRevisionNotValid = errors.ConstError("charm revision not valid")

	// CharmMetadataNotValid describes an error that occurs when the charm metadata
	// is not valid.
	CharmMetadataNotValid = errors.ConstError("charm metadata not valid")

	// CharmManifestNotValid describes an error that occurs when the charm manifest
	// is not valid.
	CharmManifestNotValid = errors.ConstError("charm manifest not valid")

	// CharmBaseNameNotValid describes an error that occurs when the charm base
	// name is not valid.
	CharmBaseNameNotValid = errors.ConstError("charm base name not valid")

	// CharmBaseNameNotSupported describes an error that occurs when the charm
	// base name is not supported.
	CharmBaseNameNotSupported = errors.ConstError("charm base name not supported")

	// ResourceNotFound describes an error that occurs when a resource is
	// not found.
	ResourceNotFound = errors.ConstError("resource not found")

	// UnknownResourceType describes an error where the resource type is
	// not oci-image or file.
	UnknownResourceType = errors.ConstError("unknown resource type resource")

	// ResourceNameNotValid describes an error where the resource name is not
	// valid, usually because it's empty.
	ResourceNameNotValid = errors.ConstError("resource name not valid")
)
