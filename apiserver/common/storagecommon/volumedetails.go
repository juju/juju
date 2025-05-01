// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/life"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// VolumeDetails returns the volume and its attachments as a params VolumeDetails.
func VolumeDetails(
	ctx context.Context,
	sb DetailsBackend,
	blockDeviceGetter BlockDeviceGetter,
	unitToMachine UnitAssignedMachineFunc,
	v state.Volume,
	attachments []state.VolumeAttachment,
) (*params.VolumeDetails, error) {
	details := &params.VolumeDetails{
		VolumeTag: v.VolumeTag().String(),
		Life:      life.Value(v.Life().String()),
	}

	if info, err := v.Info(); err == nil {
		details.Info = VolumeInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.VolumeAttachmentDetails, len(attachments))
		details.UnitAttachments = make(map[string]params.VolumeAttachmentDetails, len(attachments))
		for _, attachment := range attachments {
			attDetails := params.VolumeAttachmentDetails{
				Life: life.Value(attachment.Life().String()),
			}
			if stateInfo, err := attachment.Info(); err == nil {
				attDetails.VolumeAttachmentInfo = VolumeAttachmentInfoFromState(
					stateInfo,
				)
			}
			if attachment.Host().Kind() == names.MachineTagKind {
				details.MachineAttachments[attachment.Host().String()] = attDetails
			} else {
				details.UnitAttachments[attachment.Host().String()] = attDetails
			}
		}
	}

	aStatus, err := v.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(aStatus)

	if storageTag, err := v.StorageInstance(); err == nil {
		storageInstance, err := sb.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := StorageDetails(ctx, sb, blockDeviceGetter, unitToMachine, storageInstance)
		if err != nil {
			return nil, internalerrors.Errorf("getting storage details for volume: %w", err)
		}
		details.Storage = storageDetails
	}

	return details, nil
}
