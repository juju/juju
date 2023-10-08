// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// FilesystemDetails returns the filesystem and its attachments as a params FilesystemDetails.
func FilesystemDetails(
	sb DetailsBackend,
	unitToMachine UnitAssignedMachineFunc,
	f state.Filesystem,
	attachments []state.FilesystemAttachment,
) (*params.FilesystemDetails, error) {
	details := &params.FilesystemDetails{
		FilesystemTag: f.FilesystemTag().String(),
		Life:          life.Value(f.Life().String()),
	}

	if volumeTag, err := f.Volume(); err == nil {
		details.VolumeTag = volumeTag.String()
	}

	if info, err := f.Info(); err == nil {
		details.Info = FilesystemInfoFromState(info)
	}

	if len(attachments) > 0 {
		details.MachineAttachments = make(map[string]params.FilesystemAttachmentDetails, len(attachments))
		details.UnitAttachments = make(map[string]params.FilesystemAttachmentDetails, len(attachments))
		for _, attachment := range attachments {
			attDetails := params.FilesystemAttachmentDetails{
				Life: life.Value(attachment.Life().String()),
			}
			if stateInfo, err := attachment.Info(); err == nil {
				attDetails.FilesystemAttachmentInfo = FilesystemAttachmentInfoFromState(
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

	aStatus, err := f.Status()
	if err != nil {
		return nil, errors.Trace(err)
	}
	details.Status = common.EntityStatusFromState(aStatus)

	if storageTag, err := f.Storage(); err == nil {
		storageInstance, err := sb.StorageInstance(storageTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		storageDetails, err := StorageDetails(sb, unitToMachine, storageInstance)
		if err != nil {
			return nil, errors.Trace(err)
		}
		details.Storage = storageDetails
	}

	return details, nil
}
