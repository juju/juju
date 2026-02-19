// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestNamesValues(c *tc.C) {
	n := Names{"a", "b", "c", "a", ""}
	c.Assert(n.Values(), tc.SameContents, []string{"a", "b", "c"})
}

func (s *typesSuite) TestProvidersValues(c *tc.C) {
	p := Providers{"x", "y", "z", "x", ""}
	c.Assert(p.Values(), tc.SameContents, []string{"x", "y", "z"})
}

func (s *typesSuite) TestImportStorageInstanceParamsValidate(c *tc.C) {
	for _, test := range []struct {
		name   string
		params ImportStorageInstanceParams
		valid  bool
	}{
		{
			name: "valid params",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  1024,
				PoolName:          "default",
				UnitName:          "unit/0",
				AttachedUnitNames: []string{"unit/0", "unit/1"},
			},
			valid: true,
		},
		{
			name: "invalid params with empty pool name",
			params: ImportStorageInstanceParams{
				StorageName:       "foop",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  1024,
				PoolName:          "",
				UnitName:          "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with zero requested size",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  0,
				PoolName:          "default",
				UnitName:          "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with empty storage ID",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "",
				RequestedSizeMiB:  1024,
				PoolName:          "default",
				UnitName:          "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with invalid unit name",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  1024,
				PoolName:          "default",
				UnitName:          "invalid unit name",
			},
			valid: false,
		},
		{
			name: "invalid params with invalid pool name",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  1024,
				PoolName:          "invalid pool name",
				UnitName:          "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with invalid attachment",
			params: ImportStorageInstanceParams{
				StorageName:       "foo",
				StorageKind:       "block",
				StorageInstanceID: "foo/0",
				RequestedSizeMiB:  1024,
				PoolName:          "default",
				UnitName:          "unit/0",
				AttachedUnitNames: []string{"invalid attachment"},
			},
			valid: false,
		},
	} {
		c.Logf("testing: %s", test.name)
		err := test.params.Validate()
		if test.valid {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.NotNil)
		}
	}
}

func (s *typesSuite) TestImportFilesystemParamsValidate(c *tc.C) {
	for _, test := range []struct {
		name   string
		params ImportFilesystemParams
		valid  bool
	}{
		{
			name: "valid params",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						HostMachineName: "0",
						MountPoint:      "/mnt/fs-0",
						ReadOnly:        false,
					},
					{
						HostUnitName: "unit/0",
						MountPoint:   "/mnt/fs-0",
						ReadOnly:     true,
					},
				},
			},
			valid: true,
		}, {
			name: "invalid params with empty ID",
			params: ImportFilesystemParams{
				ID:                "",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
			},
			valid: false,
		}, {
			name: "invalid params with invalid pool name",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "invalid pool name",
				StorageInstanceID: "foo/0",
			},
			valid: false,
		}, {
			name: "invalid params with invalid storage instance ID",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "invalid storage instance id",
			},
			valid: false,
		}, {
			name: "invalid params attachment host machine and unit",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						HostMachineName: "0",
						HostUnitName:    "unit/0",
						MountPoint:      "/mnt/fs-0",
						ReadOnly:        false,
					},
				},
			},
			valid: false,
		},
		{
			name: "invalid params attachment no host",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						MountPoint: "/mnt/fs-0",
						ReadOnly:   false,
					},
				},
			},
			valid: false,
		},
		{
			name: "invalid params attachment host unit invalid",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						HostUnitName: "invalid host unit name",
						MountPoint:   "/mnt/fs-0",
						ReadOnly:     false,
					},
				},
			},
			valid: false,
		}, {
			name: "invalid params attachment host machine invalid",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						HostMachineName: "invalid host machine name",
						MountPoint:      "/mnt/fs-0",
						ReadOnly:        false,
					},
				},
			},
			valid: false,
		}, {
			name: "invalid params attachment empty mount point",
			params: ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []ImportFilesystemAttachmentsParams{
					{
						HostMachineName: "0",
						ReadOnly:        false,
					},
				},
			},
			valid: false,
		},
	} {
		c.Logf("testing: %s", test.name)
		err := test.params.Validate()
		if test.valid {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.NotNil)
		}
	}
}

func (s *typesSuite) TestImportVolumeParamsValidateNoID(c *tc.C) {
	params := ImportVolumeParams{}
	c.Assert(params.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeParamsValidateNoSize(c *tc.C) {
	params := ImportVolumeParams{
		ID: "multi-fs/0",
	}
	c.Assert(params.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeParamsValidateInvalidPoolName(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
	}
	c.Assert(params.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeParamsValidateInvalidAttachMachineNorUnit(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
		Pool:    "ebs",
		Attachments: []ImportVolumeAttachmentParams{
			{},
		},
	}
	c.Assert(params.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeParamsValidateInvalidAttachBlockDevice(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
		Pool:    "ebs",
		Attachments: []ImportVolumeAttachmentParams{
			{
				HostMachineName: "42",
			},
		},
	}
	c.Assert(params.Validate(), tc.ErrorIsNil)
}

func (s *typesSuite) TestImportVolumeParamsValidateInvalidProvisionedAttachBlockDevice(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
		Pool:    "ebs",
		Attachments: []ImportVolumeAttachmentParams{
			{
				Provisioned:     true,
				HostMachineName: "42",
			},
		},
	}
	c.Assert(params.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeAttachmentValidateInvalidUnit(c *tc.C) {
	attach := ImportVolumeAttachmentParams{
		HostUnitName: "bad-unit-id",
	}
	c.Assert(attach.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeAttachmentValidateInvalidMachine(c *tc.C) {
	attach := ImportVolumeAttachmentParams{
		HostMachineName: "bad-machine-id",
	}
	c.Assert(attach.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeAttachmentValidateProvisioned(c *tc.C) {
	attach := ImportVolumeAttachmentParams{
		HostMachineName: "42",
		Provisioned:     true,
		DeviceName:      "xvdf",
		DeviceLink:      "long-device-link-name",
	}
	c.Assert(attach.Validate(), tc.ErrorIsNil)
}

func (s *typesSuite) TestImportVolumeAttachmentValidateInvalidProvisioned(c *tc.C) {
	// Provisioned requires a device name and link.
	attach := ImportVolumeAttachmentParams{
		HostMachineName: "42",
		Provisioned:     true,
		DeviceName:      "xvdf",
	}
	c.Assert(attach.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestImportVolumeParamsValidateValidAttachment(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
		Pool:    "ebs",
		Attachments: []ImportVolumeAttachmentParams{
			{
				Provisioned:     true,
				HostMachineName: "42",
				DeviceName:      "xvdf",
				DeviceLink:      "long-device-link-name",
			},
		},
	}
	c.Assert(params.Validate(), tc.ErrorIsNil)
}

func (s *typesSuite) TestImportVolumeParamsValidateNoAttachment(c *tc.C) {
	params := ImportVolumeParams{
		ID:      "multi-fs/0",
		SizeMiB: 1024,
		Pool:    "ebs",
	}
	c.Assert(params.Validate(), tc.ErrorIsNil)
}

func (s *typesSuite) TestImportVolumeAttachmentPlanValidate(c *tc.C) {
	plan := ImportVolumeAttachmentPlanParams{
		HostMachineName: "42",
		DeviceType:      "local",
	}
	c.Assert(plan.Validate(), tc.ErrorIsNil)
}

func (s *typesSuite) TestTestImportVolumeAttachmentPlanValidateInvalidMachine(c *tc.C) {
	plan := ImportVolumeAttachmentPlanParams{
		HostMachineName: "testing",
		DeviceType:      "local",
	}
	c.Assert(plan.Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *typesSuite) TestTestImportVolumeAttachmentPlanValidateInvalidDevice(c *tc.C) {
	plan := ImportVolumeAttachmentPlanParams{
		HostMachineName: "42",
		DeviceType:      "testing",
	}
	c.Assert(plan.Validate(), tc.ErrorIs, coreerrors.NotValid)
}
