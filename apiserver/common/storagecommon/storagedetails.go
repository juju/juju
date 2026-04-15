// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// DetailsBackend is used by StorageDetails, VolumeDetails and FilesystemDetails to access
// state for collecting all the required information to send back over the wire.
type DetailsBackend interface {
	StorageAccess
	VolumeAccess
	FilesystemAccess
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)
}

// Unit represents a minimal interface for a Juju unit, exposing only the
// methods needed to determine machine assignment status for storage
// provisioning decisions.
type Unit interface {
	AssignedMachineId() (string, error)
	ShouldBeAssigned() bool
}

// UnitAssignedMachineFunc is a function type that resolves a unit tag to the
// machine tag of the machine the unit is assigned to.
type UnitAssignedMachineFunc func(names.UnitTag) (names.MachineTag, error)

// GetUnitFunc is a function type that retrieves a Unit by its name
type GetUnitFunc func(name string) (Unit, error)

// StorageDetails returns the storage instance as a params StorageDetails.
func StorageDetails(
	sb DetailsBackend,
	unitToMachine UnitAssignedMachineFunc,
	si state.StorageInstance,
	getUnit GetUnitFunc,
) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity status.StatusGetter
	var aStatus status.StatusInfo
	since := time.Now()
	if si.Kind() == state.StorageKindFilesystem {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, fsErr := sb.StorageInstanceFilesystem(si.StorageTag())
		if errors.Is(fsErr, errors.NotFound) {
			var err error
			aStatus, err = missingBackingStorageStatus(
				si, getUnit, fsErr,
				"waiting for filesystem to be provisioned", since,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else if fsErr != nil {
			return nil, errors.Trace(fsErr)
		} else {
			statusEntity = filesystem
		}
	} else {
		volume, volErr := sb.StorageInstanceVolume(si.StorageTag())
		if errors.Is(volErr, errors.NotFound) {
			var err error
			aStatus, err = missingBackingStorageStatus(
				si, getUnit, volErr,
				"waiting for volume to be provisioned", since,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else if volErr != nil {
			return nil, errors.Trace(volErr)
		} else {
			statusEntity = volume
			if info, err := volume.Info(); err == nil {
				persistent = info.Persistent
			}
		}

	}
	if statusEntity != nil {
		var err error
		aStatus, err = statusEntity.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Get unit storage attachments.
	storageAttachments, err := sb.StorageAttachments(si.StorageTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	var storageAttachmentDetails map[string]params.StorageAttachmentDetails
	if len(storageAttachments) > 0 {
		storageAttachmentDetails = make(map[string]params.StorageAttachmentDetails)
		for _, a := range storageAttachments {
			// TODO(caas) - handle attachments to units
			machineTag, location, err := storageAttachmentInfo(sb, a, unitToMachine)
			if err != nil {
				return nil, errors.Trace(err)
			}
			details := params.StorageAttachmentDetails{
				StorageTag: a.StorageInstance().String(),
				UnitTag:    a.Unit().String(),
				Location:   location,
				Life:       life.Value(a.Life().String()),
			}
			if machineTag.Id() != "" {
				details.MachineTag = machineTag.String()
			}
			storageAttachmentDetails[a.Unit().String()] = details
		}
	}

	var ownerTag string
	if owner, ok := si.Owner(); ok {
		ownerTag = owner.String()
	}

	return &params.StorageDetails{
		StorageTag:  si.Tag().String(),
		OwnerTag:    ownerTag,
		Kind:        params.StorageKind(si.Kind()),
		Life:        life.Value(si.Life().String()),
		Status:      common.EntityStatusFromState(aStatus),
		Persistent:  persistent,
		Attachments: storageAttachmentDetails,
	}, nil
}

func storageAttachmentInfo(
	sb DetailsBackend,
	a state.StorageAttachment,
	unitToMachine UnitAssignedMachineFunc,
) (_ names.MachineTag, location string, _ error) {
	machineTag, err := unitToMachine(a.Unit())
	if errors.Is(err, errors.NotAssigned) {
		return names.MachineTag{}, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	info, err := StorageAttachmentInfo(sb, sb, sb, a, machineTag)
	if errors.Is(err, errors.NotProvisioned) {
		return machineTag, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	return machineTag, info.Location, nil
}

// missingBackingStorageStatus classifies storage status when the backing
// volume/filesystem record is missing.
func missingBackingStorageStatus(
	si state.StorageInstance,
	getUnit GetUnitFunc,
	notFoundErr error,
	message string,
	since time.Time,
) (status.StatusInfo, error) {
	owner, hasOwner := si.Owner()
	if !hasOwner || owner.Kind() != names.UnitTagKind {
		return status.StatusInfo{
			Status:  status.Error,
			Message: notFoundErr.Error(),
			Since:   &since,
		}, nil
	}
	unitTag, err := names.ParseUnitTag(owner.String())
	if err != nil {
		return status.StatusInfo{}, err
	}

	unit, err := getUnit(unitTag.Id())
	if err != nil {
		return status.StatusInfo{}, err
	}

	// CAAS units are never assigned to machines. Storage backing for CAAS
	// is created in the same transaction as the storage instance, so a
	// missing backing record indicates a real error.
	if !unit.ShouldBeAssigned() {
		return status.StatusInfo{
			Status:  status.Error,
			Message: notFoundErr.Error(),
			Since:   &since,
		}, nil
	}

	_, err = unit.AssignedMachineId()
	switch {
	case errors.Is(err, errors.NotAssigned):
		// This is a valid condition in which the storage backing is not created yet
		// until the unit is assigned to a machine. We treat this as pending due to
		// eventual consistency.
		return status.StatusInfo{
			Status:  status.Pending,
			Message: message,
			Since:   &since,
		}, nil
	case err != nil:
		// This must be some other error, so surface it to the caller.
		return status.StatusInfo{}, err
	default:
		// When a unit is assigned to a machine, the machine assignment
		// transaction [AssignToMachine] func guarantees that the backing
		// volume/filesystem document is created. A NotFound error when the unit
		// is assigned indicates something has gone wrong, so propagate this error as
		// part of StatusInfo to keep status command from error-ing.
		return status.StatusInfo{
			Status:  status.Error,
			Message: notFoundErr.Error(),
			Since:   &since,
		}, nil
	}
}
