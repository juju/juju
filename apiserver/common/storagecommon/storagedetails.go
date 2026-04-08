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

// DetailsBacked is used by StorageDetails, VolumeDetails and FilesystemDetails to access
// state for collecting all the required information to send back over the wire.
type DetailsBackend interface {
	StorageAccess
	VolumeAccess
	FilesystemAccess
	StorageAttachments(names.StorageTag) ([]state.StorageAttachment, error)
}

type UnitAssignedMachineFunc func(names.UnitTag) (names.MachineTag, error)

// StorageDetails returns the storage instance as a params StorageDetails.
func StorageDetails(
	sb DetailsBackend,
	unitToMachine UnitAssignedMachineFunc,
	si state.StorageInstance,
) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity status.StatusGetter
	var aStatus status.StatusInfo
	since := time.Now()
	storageAttachments, err := sb.StorageAttachments(si.StorageTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if si.Kind() == state.StorageKindFilesystem {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, fsErr := sb.StorageInstanceFilesystem(si.StorageTag())
		if errors.Is(fsErr, errors.NotFound) {
			var err error
			aStatus, err = missingBackingStorageStatus(
				storageAttachments, unitToMachine, fsErr,
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
				storageAttachments, unitToMachine, volErr,
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
	attachments []state.StorageAttachment,
	unitToMachine UnitAssignedMachineFunc,
	notFoundErr error,
	message string,
	since time.Time,
) (status.StatusInfo, error) {
	if len(attachments) == 0 {
		return status.StatusInfo{
			Status: status.Detached,
			Since:  &since,
		}, nil
	}
	assigned, err := allAttachedUnitsAssigned(attachments, unitToMachine)
	if err != nil {
		return status.StatusInfo{}, err
	}
	// When a unit is assigned to a machine, the machine assignment
	// transaction [AssignToMachine] func guarantees that the backing
	// volume/filesystem document is created. A NotFound error when the unit
	// is assigned indicates something has gone wrong, so propagate this error as
	// part of StatusInfo to keep status command from error-ing.
	if assigned {
		return status.StatusInfo{
			Status:  status.Error,
			Message: notFoundErr.Error(),
			Since:   &since,
		}, nil
	}
	return status.StatusInfo{
		Status:  status.Pending,
		Message: message,
		Since:   &since,
	}, nil
}

// allAttachedUnitsAssigned returns true if all units in attachments are
// assigned to a machine.
func allAttachedUnitsAssigned(
	attachments []state.StorageAttachment,
	unitToMachine UnitAssignedMachineFunc,
) (bool, error) {
	for _, a := range attachments {
		_, err := unitToMachine(a.Unit())
		if errors.Is(err, errors.NotAssigned) {
			return false, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}

	}
	return true, nil
}
