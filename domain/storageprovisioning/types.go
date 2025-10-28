// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storage"
)

// ApplicationResourceTagInfo represents storage tag information in the context
// of an application.
type ApplicationResourceTagInfo struct {
	ApplicationName string
	ModelResourceTagInfo
}

// ModelResourceTagInfo represents storage tag information from the model.
type ModelResourceTagInfo struct {
	BaseResourceTags string
	ModelUUID        string
	ControllerUUID   string
}

// PlanDeviceType defines what type of storage attachment plan is required.
type PlanDeviceType int

const (
	// PlanDeviceTypeLocal indicates a local attachment.
	PlanDeviceTypeLocal PlanDeviceType = iota
	// PlanDeviceTypeISCSI indicates an iscsi attachment.
	PlanDeviceTypeISCSI
)

// StorageAttachmentInfo represents information about a storage attachment.
type StorageAttachmentInfo struct {
	Kind                 storage.StorageKind
	Life                 life.Life
	FilesystemMountPoint string
	BlockDeviceUUID      blockdevice.BlockDeviceUUID
}
