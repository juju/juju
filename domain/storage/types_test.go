// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestNamesValues(c *tc.C) {
	n := storage.Names{"a", "b", "c", "a", ""}
	c.Assert(n.Values(), tc.SameContents, []string{"a", "b", "c"})
}

func (s *typesSuite) TestProvidersValues(c *tc.C) {
	p := storage.Providers{"x", "y", "z", "x", ""}
	c.Assert(p.Values(), tc.SameContents, []string{"x", "y", "z"})
}

func (s *typesSuite) TestImportStorageInstanceParamsValidate(c *tc.C) {
	for _, test := range []struct {
		name   string
		params storage.ImportStorageInstanceParams
		valid  bool
	}{
		{
			name: "valid params",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foo",
				StorageKind:      "block",
				StorageID:        "foo/0",
				RequestedSizeMiB: 1024,
				PoolName:         "default",
				UnitName:         "unit/0",
			},
			valid: true,
		},
		{
			name: "invalid params with empty pool name",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foop",
				StorageKind:      "block",
				StorageID:        "foo/0",
				RequestedSizeMiB: 1024,
				PoolName:         "",
				UnitName:         "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with zero requested size",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foo",
				StorageKind:      "block",
				StorageID:        "foo/0",
				RequestedSizeMiB: 0,
				PoolName:         "default",
				UnitName:         "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with empty storage ID",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foo",
				StorageKind:      "block",
				StorageID:        "",
				RequestedSizeMiB: 1024,
				PoolName:         "default",
				UnitName:         "unit/0",
			},
			valid: false,
		},
		{
			name: "invalid params with invalid unit name",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foo",
				StorageKind:      "block",
				StorageID:        "foo/0",
				RequestedSizeMiB: 1024,
				PoolName:         "default",
				UnitName:         "invalid unit name",
			},
			valid: false,
		},
		{
			name: "invalid params with invalid pool name",
			params: storage.ImportStorageInstanceParams{
				StorageName:      "foo",
				StorageKind:      "block",
				StorageID:        "foo/0",
				RequestedSizeMiB: 1024,
				PoolName:         "invalid pool name",
				UnitName:         "unit/0",
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
		params storage.ImportFilesystemParams
		valid  bool
	}{
		{
			name: "valid params",
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
			params: storage.ImportFilesystemParams{
				ID:                "",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
			},
			valid: false,
		}, {
			name: "invalid params with invalid pool name",
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "invalid pool name",
				StorageInstanceID: "foo/0",
			},
			valid: false,
		}, {
			name: "invalid params with invalid storage instance ID",
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "invalid storage instance id",
			},
			valid: false,
		}, {
			name: "invalid params attachment host machine and unit",
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
			params: storage.ImportFilesystemParams{
				ID:                "fs-0",
				SizeInMiB:         1024,
				ProviderID:        "provider-id",
				PoolName:          "default",
				StorageInstanceID: "foo/0",
				Attachments: []storage.ImportFilesystemAttachmentsParams{
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
