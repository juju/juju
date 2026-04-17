// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
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

	attachInfo := domainstorage.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfoForAttach: domainstorage.StorageInstanceInfoForAttach{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:      storageUUID,
				CharmName: nil,
				Filesystem: &domainstorage.StorageInstanceAttachFilesystemInfo{
					UUID: filesystemUUID,
				},
				Volume: &domainstorage.StorageInstanceAttachVolumeInfo{
					UUID: volumeUUID,
				},
				StorageName: "data",
			},
			StorageInstanceAttachments: []domainstorage.StorageInstanceUnitAttachmentID{
				{
					UUID:     attachmentUUID1,
					UnitUUID: unitUUID,
				},
				{
					UUID:     attachmentUUID2,
					UnitUUID: unitUUID,
				},
			},
		},
		UnitAttachNamedStorageInfo: domainstorage.UnitAttachNamedStorageInfo{
			AlreadyAttachedCount: 2,
			CharmMetadataName:    "example",
			CharmUUID:            charmUUID,
			MachineUUID:          &machineUUID,
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

	expected := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: storageUUID,
			FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: filesystemUUID,
				NetNodeUUID:    netNodeUUID,
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
			VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
				NetNodeUUID:    netNodeUUID,
				ProvisionScope: domainstorage.ProvisionScopeModel,
				VolumeUUID:     volumeUUID,
			},
		},
		StorageInstanceAttachmentCheckArgs: domainstorage.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
				attachmentUUID1,
				attachmentUUID2,
			},
			UUID: storageUUID,
		},
		StorageInstanceCharmNameSetArg: &domainstorage.StorageInstanceCharmNameSetArg{
			CharmMetadataName: "example",
			UUID:              storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: domainstorage.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 2,
			CharmUUID:          charmUUID,
			MachineUUID:        &machineUUID,
		},
	}

	checker := tc.NewMultiChecker()
	checker.AddExpr("_.CreateUnitStorageAttachmentArg.UUID", tc.IsNonZeroUUID)
	checker.AddExpr(
		"_.CreateUnitStorageAttachmentArg.FilesystemAttachment.UUID",
		tc.IsNonZeroUUID,
	)
	checker.AddExpr(
		"_.CreateUnitStorageAttachmentArg.VolumeAttachment.UUID",
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

	attachInfo := domainstorage.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfoForAttach: domainstorage.StorageInstanceInfoForAttach{
			StorageInstanceAttachInfo: domainstorage.StorageInstanceAttachInfo{
				UUID:        storageUUID,
				CharmName:   &charmName,
				StorageName: "data",
			},
		},
		UnitAttachNamedStorageInfo: domainstorage.UnitAttachNamedStorageInfo{
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

	expected := domainstorage.AttachStorageInstanceToUnitArg{
		CreateUnitStorageAttachmentArg: domainstorage.CreateUnitStorageAttachmentArg{
			StorageInstanceUUID: storageUUID,
		},
		StorageInstanceAttachmentCheckArgs: domainstorage.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{},
			UUID:                storageUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: domainstorage.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: attachInfo.UnitAttachNamedStorageInfo.AlreadyAttachedCount,
			CharmUUID:          charmUUID,
			MachineUUID:        nil,
		},
	}

	checker := tc.NewMultiChecker()
	checker.AddExpr("_.CreateUnitStorageAttachmentArg.UUID", tc.IsNonZeroUUID)

	c.Check(arg, checker, expected)
}
