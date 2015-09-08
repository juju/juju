// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/common"
)

const filesystemCmdDoc = `
"juju storage filesystem" is used to manage storage filesystems in
 the Juju environment.
`

const filesystemCmdPurpose = "manage storage filesystems"

// NewFilesystemSuperCommand creates the storage filesystem super subcommand and
// registers the subcommands that it supports.
func NewFilesystemSuperCommand() cmd.Command {
	poolcmd := jujucmd.NewSubSuperCommand(cmd.SuperCommandParams{
		Name:        "filesystem",
		Doc:         filesystemCmdDoc,
		UsagePrefix: "juju storage",
		Purpose:     filesystemCmdPurpose,
	})
	poolcmd.Register(envcmd.Wrap(&FilesystemListCommand{}))
	return poolcmd
}

// FilesystemCommandBase is a helper base structure for filesystem commands.
type FilesystemCommandBase struct {
	StorageCommandBase
}

// FilesystemInfo defines the serialization behaviour for storage filesystem.
type FilesystemInfo struct {
	// from params.Filesystem. This is provider-supplied unique filesystem id.
	FilesystemId string `yaml:"id,omitempty" json:"id,omitempty"`

	// from params.FilesystemInfo
	Size uint64 `yaml:"size" json:"size"`

	// from params.FilesystemAttachmentInfo
	MountPoint string `yaml:"mountpoint,omitempty" json:"mountpoint,omitempty"`

	// from params.FilesystemAttachmentInfo
	ReadOnly bool `yaml:"read-only" json:"read-only"`

	// from params.FilesystemInfo. This is juju-supplied filesystem ID.
	Filesystem string `yaml:"filesystem" json:"filesystem"`

	// from params.FilesystemDetails. This is the juju-supplied ID of the
	// volume backing the filesystem if any.
	Volume string `yaml:"volume,omitempty" json:"volume,omitempty"`

	// from params.FilesystemInfo.
	Status EntityStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// convertToFilesystemInfo returns map of maps with filesystem info
// keyed first on machine ID and then on filesystem ID.
func convertToFilesystemInfo(all []params.FilesystemDetailsResult) (map[string]map[string]map[string]FilesystemInfo, error) {
	result := map[string]map[string]map[string]FilesystemInfo{}
	for _, one := range all {
		if err := convertFilesystemDetailsResult(one, result); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

func convertFilesystemDetailsResult(item params.FilesystemDetailsResult, all map[string]map[string]map[string]FilesystemInfo) error {
	info, attachments, storage, storageOwner, err := createFilesystemInfo(item)
	if err != nil {
		return errors.Trace(err)
	}
	for machineTag, attachmentInfo := range attachments {
		machineId, err := idFromTag(machineTag)
		if err != nil {
			return errors.Trace(err)
		}
		info.MountPoint = attachmentInfo.MountPoint
		info.ReadOnly = attachmentInfo.ReadOnly
		addOneFilesystemToAll(machineId, storage, storageOwner, info, all)
	}
	if len(attachments) == 0 {
		addOneFilesystemToAll("unattached", storage, storageOwner, info, all)
	}
	return nil
}

func addOneFilesystemToAll(machineId, storageId, storageOwnerId string, item FilesystemInfo, all map[string]map[string]map[string]FilesystemInfo) {
	machineFilesystems, ok := all[machineId]
	if !ok {
		machineFilesystems = map[string]map[string]FilesystemInfo{}
		all[machineId] = machineFilesystems
	}
	storageOwnerFilesystems, ok := machineFilesystems[storageOwnerId]
	if !ok {
		storageOwnerFilesystems = map[string]FilesystemInfo{}
		machineFilesystems[storageOwnerId] = storageOwnerFilesystems
	}
	storageOwnerFilesystems[storageId] = item
}

func createFilesystemInfo(result params.FilesystemDetailsResult) (
	info FilesystemInfo,
	attachments map[string]params.FilesystemAttachmentInfo,
	storageId string,
	storageOwnerId string,
	err error,
) {
	info.FilesystemId = result.Result.Info.FilesystemId
	info.Size = result.Result.Info.Size
	info.Status = EntityStatus{
		result.Result.Status.Status,
		result.Result.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(result.Result.Status.Since, false),
	}

	if f, err := idFromTag(result.Result.FilesystemTag); err != nil {
		return FilesystemInfo{}, nil, "", "", errors.Trace(err)
	} else {
		info.Filesystem = f
	}

	if result.Result.VolumeTag != "" {
		if v, err := idFromTag(result.Result.VolumeTag); err == nil {
			info.Volume = v
		}
	}

	storageId = "unassigned"
	if result.Result.StorageTag != "" {
		if storageId, err = idFromTag(result.Result.StorageTag); err != nil {
			return FilesystemInfo{}, nil, "", "", errors.Trace(err)
		}
	}

	storageOwnerId = "unattached"
	if result.Result.StorageOwnerTag != "" {
		if storageOwnerId, err = idFromTag(result.Result.StorageOwnerTag); err != nil {
			return FilesystemInfo{}, nil, "", "", errors.Trace(err)
		}
	}

	attachments = result.Result.MachineAttachments
	return info, attachments, storageId, storageOwnerId, nil
}
