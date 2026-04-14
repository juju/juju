// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	storageprovisioning "github.com/juju/juju/domain/storageprovisioning"
)

// attachStorageArgSuite tests MakeAttachStorageInstanceToUnitArg behaviour.
type attachStorageArgSuite struct{}

// TestAttachStorageArgSuite runs the tests defined in [attachStorageArgSuite].
func TestAttachStorageArgSuite(t *testing.T) {
	tc.Run(t, &attachStorageArgSuite{})
}

// TestMakeAttachStorageInstanceToUnitArgFull asserts a full attachment arg is
// constructed from the provided attachment info.
func (s *attachStorageArgSuite) TestMakeAttachStorageInstanceToUnitArgFull(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	charmUUID := tc.Must(c, corecharm.NewID)
	machineUUID := tc.Must(c, coremachine.NewUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	attachmentUUID1 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	attachmentUUID2 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)

	attachInfo := internal.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfo: internal.StorageInstanceInfo{
			UUID:        storageUUID,
			CharmName:   nil,
			Filesystem:  &internal.StorageInstanceFilesystemInfo{UUID: filesystemUUID},
			Volume:      &internal.StorageInstanceVolumeInfo{UUID: volumeUUID},
			StorageName: "data",
		},
		UnitNamedStorageInfo: internal.UnitNamedStorageInfo{
			AlreadyAttachedCount: 2,
			CharmMetadataName:    "example",
			CharmUUID:            charmUUID,
			MachineUUID:          &machineUUID,
			NetNodeUUID:          netNodeUUID,
			UUID:                 unitUUID,
		},
		StorageInstanceAttachments: []internal.StorageInstanceUnitAttachment{
			{UUID: attachmentUUID1},
			{UUID: attachmentUUID2},
		},
	}

	svc := Service{}
	arg, err := svc.MakeAttachStorageInstanceToUnitArg(
		c.Context(),
		attachInfo,
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageUUID,
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: filesystemUUID,
				NetNodeUUID:    netNodeUUID,
				ProvisionScope: storageprovisioning.ProvisionScopeModel,
			},
			VolumeAttachment: &internal.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    netNodeUUID,
				ProvisionScope: storageprovisioning.ProvisionScopeModel,
				VolumeUUID:     volumeUUID,
			},
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
				attachmentUUID1,
				attachmentUUID2,
			},
			UUID: storageUUID,
		},
		StorageInstanceCharmNameSetArg: &internal.StorageInstanceCharmNameSetArg{
			CharmMetadataName: "example",
			UUID:              storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 2,
			CharmUUID:          charmUUID,
			MachineUUID:        &machineUUID,
		},
	}

	checker := tc.NewMultiChecker()
	checker.AddExpr("_.CreateStorageInstanceAttachmentArg.UUID", tc.IsNonZeroUUID)
	checker.AddExpr(
		"_.CreateStorageInstanceAttachmentArg.FilesystemAttachment.UUID",
		tc.IsNonZeroUUID,
	)
	checker.AddExpr(
		"_.CreateStorageInstanceAttachmentArg.VolumeAttachment.UUID",
		tc.IsNonZeroUUID,
	)

	c.Check(arg, checker, expected)
}

// TestMakeAttachStorageInstanceToUnitArgCharmSetSkipped asserts charm name set
// args are omitted when the storage instance already has one.
func (s *attachStorageArgSuite) TestMakeAttachStorageInstanceToUnitArgCharmSetSkipped(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	charmUUID := tc.Must(c, corecharm.NewID)
	charmName := "example"

	attachInfo := internal.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfo: internal.StorageInstanceInfo{
			UUID:        storageUUID,
			CharmName:   &charmName,
			StorageName: "data",
		},
		UnitNamedStorageInfo: internal.UnitNamedStorageInfo{
			AlreadyAttachedCount: 0,
			CharmMetadataName:    charmName,
			CharmUUID:            charmUUID,
			NetNodeUUID:          netNodeUUID,
			UUID:                 unitUUID,
		},
	}

	svc := Service{}
	arg, err := svc.MakeAttachStorageInstanceToUnitArg(
		c.Context(),
		attachInfo,
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{},
			UUID:                storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: attachInfo.UnitNamedStorageInfo.AlreadyAttachedCount,
			CharmUUID:          charmUUID,
			MachineUUID:        nil,
		},
	}

	checker := tc.NewMultiChecker()
	checker.AddExpr("_.CreateStorageInstanceAttachmentArg.UUID", tc.IsNonZeroUUID)

	c.Check(arg, checker, expected)
}
