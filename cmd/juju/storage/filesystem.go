// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
)

// FilesystemCommandBase is a helper base structure for filesystem commands.
type FilesystemCommandBase struct {
	StorageCommandBase
}

// FilesystemInfo defines the serialization behaviour for storage filesystem.
type FilesystemInfo struct {
	// from params.Filesystem. This is provider-supplied unique filesystem id.
	ProviderFilesystemId string `yaml:"provider-id,omitempty" json:"provider-id,omitempty"`

	// Volume is the ID of the volume that the filesystem is backed by, if any.
	Volume string `yaml:"volume,omitempty" json:"volume,omitempty"`

	// Storage is the ID of the storage instance that the filesystem is
	// assigned to, if any.
	Storage string `yaml:"storage,omitempty" json:"storage,omitempty"`

	// Attachments is the set of entities attached to the filesystem.
	Attachments *FilesystemAttachments

	// Pool is the name of the storage pool that the filesystem came from.
	Pool string `yaml:"pool,omitempty" json:"pool,omitempty"`

	// from params.FilesystemInfo
	Size uint64 `yaml:"size" json:"size"`

	// Life is the lifecycle state of the filesystem.
	Life string `yaml:"life,omitempty" json:"life,omitempty"`

	// from params.FilesystemInfo.
	Status EntityStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

type FilesystemAttachments struct {
	Machines map[string]MachineFilesystemAttachment `yaml:"machines,omitempty" json:"machines,omitempty"`
	Units    map[string]UnitStorageAttachment       `yaml:"units,omitempty" json:"units,omitempty"`
}

type MachineFilesystemAttachment struct {
	MountPoint string `yaml:"mount-point" json:"mount-point"`
	ReadOnly   bool   `yaml:"read-only" json:"read-only"`
	Life       string `yaml:"life,omitempty" json:"life,omitempty"`
}

// generateListFilesystemOutput returns a map filesystem IDs to filesystem info
func generateListFilesystemsOutput(ctx *cmd.Context, api StorageListAPI, ids []string) (map[string]FilesystemInfo, error) {

	results, err := api.ListFilesystems(ids)
	if err != nil {
		return nil, err
	}

	// filter out valid output, if any
	var valid []params.FilesystemDetails
	for _, result := range results {
		if result.Error == nil {
			valid = append(valid, result.Result...)
			continue
		}
		// display individual error
		fmt.Fprintf(ctx.Stderr, "%v\n", result.Error)
	}
	if len(valid) == 0 {
		return nil, nil
	}
	return convertToFilesystemInfo(valid)
}

// convertToFilesystemInfo returns a map of filesystem IDs to filesystem info.
func convertToFilesystemInfo(all []params.FilesystemDetails) (map[string]FilesystemInfo, error) {
	result := make(map[string]FilesystemInfo)
	for _, one := range all {
		filesystemTag, info, err := createFilesystemInfo(one)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[filesystemTag.Id()] = info
	}
	return result, nil
}

func createFilesystemInfo(details params.FilesystemDetails) (names.FilesystemTag, FilesystemInfo, error) {
	filesystemTag, err := names.ParseFilesystemTag(details.FilesystemTag)
	if err != nil {
		return names.FilesystemTag{}, FilesystemInfo{}, errors.Trace(err)
	}

	var info FilesystemInfo
	info.ProviderFilesystemId = details.Info.FilesystemId
	info.Pool = details.Info.Pool
	info.Size = details.Info.Size
	info.Life = string(details.Life)
	info.Status = EntityStatus{
		details.Status.Status,
		details.Status.Info,
		// TODO(axw) we should support formatting as ISO time
		common.FormatTime(details.Status.Since, false),
	}

	if details.VolumeTag != "" {
		volumeId, err := idFromTag(details.VolumeTag)
		if err != nil {
			return names.FilesystemTag{}, FilesystemInfo{}, errors.Trace(err)
		}
		info.Volume = volumeId
	}

	if len(details.MachineAttachments) > 0 {
		machineAttachments := make(map[string]MachineFilesystemAttachment)
		for machineTag, attachment := range details.MachineAttachments {
			machineId, err := idFromTag(machineTag)
			if err != nil {
				return names.FilesystemTag{}, FilesystemInfo{}, errors.Trace(err)
			}
			machineAttachments[machineId] = MachineFilesystemAttachment{
				attachment.MountPoint,
				attachment.ReadOnly,
				string(attachment.Life),
			}
		}
		info.Attachments = &FilesystemAttachments{
			Machines: machineAttachments,
		}
	}

	if details.Storage != nil {
		storageTag, storageInfo, err := createStorageInfo(*details.Storage)
		if err != nil {
			return names.FilesystemTag{}, FilesystemInfo{}, errors.Trace(err)
		}
		info.Storage = storageTag.Id()
		if storageInfo.Attachments != nil {
			if info.Attachments == nil {
				info.Attachments = &FilesystemAttachments{}
			}
			info.Attachments.Units = storageInfo.Attachments.Units
		}
	}

	return filesystemTag, info, nil
}
