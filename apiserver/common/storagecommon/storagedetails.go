// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

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

type UnitAssignedMachineFunc func(context.Context, names.UnitTag) (names.MachineTag, error)

// StorageDetails returns the storage instance as a params StorageDetails.
func StorageDetails(
	ctx context.Context,
	sb DetailsBackend,
	blockDeviceGetter BlockDeviceGetter,
	unitToMachine UnitAssignedMachineFunc,
	si state.StorageInstance,
) (*params.StorageDetails, error) {
	// Get information from underlying volume or filesystem.
	var persistent bool
	var statusEntity status.StatusGetter
	if si.Kind() == state.StorageKindFilesystem {
		// TODO(axw) when we support persistent filesystems,
		// e.g. CephFS, we'll need to do set "persistent"
		// here too.
		filesystem, err := sb.StorageInstanceFilesystem(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusEntity = filesystem
	} else {
		volume, err := sb.StorageInstanceVolume(si.StorageTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if info, err := volume.Info(); err == nil {
			persistent = info.Persistent
		}
		statusEntity = volume
	}
	aStatus, err := statusEntity.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get unit storage attachments.
	var storageAttachmentDetails map[string]params.StorageAttachmentDetails
	storageAttachments, err := sb.StorageAttachments(si.StorageTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(storageAttachments) > 0 {
		storageAttachmentDetails = make(map[string]params.StorageAttachmentDetails)
		for _, a := range storageAttachments {
			// TODO(caas) - handle attachments to units
			machineTag, location, err := storageAttachmentInfo(ctx, sb, blockDeviceGetter, a, unitToMachine)
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
	ctx context.Context,
	sb DetailsBackend,
	blockDeviceGetter BlockDeviceGetter,
	a state.StorageAttachment,
	unitToMachine UnitAssignedMachineFunc,
) (_ names.MachineTag, location string, _ error) {
	machineTag, err := unitToMachine(ctx, a.Unit())
	if errors.Is(err, errors.NotAssigned) {
		return names.MachineTag{}, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	info, err := StorageAttachmentInfo(ctx, sb, sb, sb, blockDeviceGetter, a, machineTag)
	if errors.Is(err, errors.NotProvisioned) {
		return machineTag, "", nil
	} else if err != nil {
		return names.MachineTag{}, "", errors.Trace(err)
	}
	return machineTag, info.Location, nil
}
