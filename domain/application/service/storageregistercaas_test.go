// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	caas "github.com/juju/juju/caas"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/internal"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
)

// registerCAASStorageSuite is a suite tests concerned with testing
// functionality around storage registration for caas units.
type registerCAASStorageSuite struct{}

func TestRegisterCAASStorageSuite(t *testing.T) {
	tc.Run(t, registerCAASStorageSuite{})
}

// TestMakeCAASStorageInstanceProviderIDAssociations tests the happy path of
// [makeCAASStorageInstanceProviderIDAssociations]. This test is aimed at
// ensuring that new storage being created for a unit has a provide id assigned.
func (registerCAASStorageSuite) TestMakeCAASStorageInstanceProviderIDAssociations(c *tc.C) {
	pFSInfo := []caas.FilesystemInfo{
		{
			FilesystemId: "fs-1",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-2",
			StorageName:  "st1",
		},
		{
			FilesystemId: "fs-3",
			StorageName:  "st2",
		},
	}

	existingProviderStorage := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProviderID: "fs-2",
			},
			StorageName: "st1",
			Volume: &internal.StorageInstanceCompositionVolume{
				ProviderID: "fs-2",
			},
		},
	}

	fs1UUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	fs2UUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)
	v1UUID := tc.Must(c, domainstorageprov.NewVolumeUUID)
	unitStorageToCreate := []application.CreateUnitStorageInstanceArg{
		{
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				UUID: fs1UUID,
			},
			Name: "st1",
			Volume: &application.CreateUnitStorageVolumeArg{
				UUID: v1UUID,
			},
		},
		{
			Filesystem: &application.CreateUnitStorageFilesystemArg{
				UUID: fs2UUID,
			},
			Name: "st2",
		},
	}

	fsAssociations, vAssociations :=
		makeCAASStorageInstanceProviderIDAssociations(
			pFSInfo, existingProviderStorage, unitStorageToCreate,
		)

	c.Check(fsAssociations, tc.DeepEquals, map[domainstorageprov.FilesystemUUID]string{
		fs1UUID: "fs-1",
		fs2UUID: "fs-3",
	})
	c.Check(vAssociations, tc.DeepEquals, map[domainstorageprov.VolumeUUID]string{
		v1UUID: "fs-1",
	})
}
