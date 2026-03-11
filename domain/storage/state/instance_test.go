// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	domainlife "github.com/juju/juju/domain/life"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
)

// instanceSuite is a test suite for asserting storage instance based interfaces
// in this package.
type instanceSuite struct {
	baseSuite
}

// TestInstanceSuite runs the tests contained within [instanceSuite].
func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

// TestGetStorageInstanceUUIDByID tests the happy path of getting a storage
// innstance uuid by it's id value.
func (s *instanceSuite) TestGetStorageInstanceUUIDByID(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid, id := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token",
	)

	st := NewState(s.TxnRunnerFactory())
	gotUUID, err := st.GetStorageInstanceUUIDByID(c.Context(), id)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, uuid)
}

// TestGetStorageInstanceUUIDByIDNotFound tests the case where a storage
// instance cannot be found for a given storage id. In this case the caller MUST
// get back an error satisfying [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestGetStorageInstanceUUIDByIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceUUIDByID(c.Context(), "non-existent-id")
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDs(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token1",
	)
	uuid2, id2 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]domainstorage.StorageInstanceUUID{
		id1: uuid1,
		id2: uuid2,
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDsDuplicateIDs(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token1",
	)
	uuid2, id2 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2, id1})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]domainstorage.StorageInstanceUUID{
		id1: uuid1,
		id2: uuid2,
	})
}

func (s *instanceSuite) TestGetStorageInstanceUUIDsByIDsMiss(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "pool1", "myprovider", nil)
	uuid1, id1 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token1",
	)
	uuid2, id2 := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "token2",
	)

	st := NewState(s.TxnRunnerFactory())
	uuidMap, err := st.GetStorageInstanceUUIDsByIDs(c.Context(), []string{id1, id2, "foo", "bar"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuidMap, tc.DeepEquals, map[string]domainstorage.StorageInstanceUUID{
		id1: uuid1,
		id2: uuid2,
	})
}

// TestStorageInstanceInfoNotFound tests that when asking for Storage instance
// information for a uuid that does not exist the caller gets back an error
// statisfying [domainstorageerrors.StorageInstanceNotFound].
func (s *instanceSuite) TestStorageInstanceInfoNotFound(c *tc.C) {
	siUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestStorageInstanceInfoNoAttachments tests getting Storage instance
// information when the StorageInstance has no attachments and composition.
func (s *instanceSuite) TestStorageInstanceInfoNoAttachments(c *tc.C) {
	charmUUID := s.newCharm(c)
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)
	siUUID, siID := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "storage1",
	)

	st := NewState(s.TxnRunnerFactory())
	info, err := st.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, internal.StorageInstanceInfo{
		Attachments: []internal.StorageInstanceInfoAttachment{},
		Life:        domainlife.Alive,
		Kind:        domainstorage.StorageKindBlock,
		StorageID:   siID,
		UUID:        siUUID,
	})
}

// TestStorageInstanceInfoFilesystemUnitAttach tests that
// [State.GetStorageInstanceInfo] correctly returns storage instance information
// for a filesystem-backed storage instance that is attached to a unit. The test
// verifies that the returned info includes the filesystem details, mount point,
// attachment information, unit owner, and status.
func (s *instanceSuite) TestStorageInstanceInfoFilesystemUnitAttach(c *tc.C) {
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)
	appUUID, charmUUID := s.newApplication(c, "app1")
	siUUID, siID := s.newFilesystemStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "storage1",
	)
	unitUUID, unitName, unitNetNodeUUID := s.newUnitForApplication(c, appUUID)
	s.newStorageInstanceUnitOwner(c, siUUID, unitUUID)
	saUUID := s.newStorageAttachment(c, siUUID, unitUUID)
	fsUUID := s.newModelFilesystem(c, siUUID)
	s.newModelFilesystemAttachmentWithMountPoint(
		c, fsUUID, unitNetNodeUUID, "/mnt/fs1",
	)

	statusTime := time.Now()
	s.setFilesystemStatus(
		c, fsUUID, domainstatus.StorageFilesystemStatusTypeAttached,
		"test", statusTime,
	)

	st := NewState(s.TxnRunnerFactory())
	info, err := st.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, internal.StorageInstanceInfo{
		Attachments: []internal.StorageInstanceInfoAttachment{
			{
				Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
					MountPoint: "/mnt/fs1",
				},
				Life:     domainlife.Alive,
				UnitName: unitName,
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		Filesystem: &internal.StorageInstanceInfoFilesystem{
			Status: &internal.StorageInstanceInfoFilesystemStatus{
				Message:   "test",
				Status:    domainstatus.StorageFilesystemStatusTypeAttached,
				UpdatedAt: &statusTime,
			},
			UUID: fsUUID,
		},
		Life:      domainlife.Alive,
		Kind:      domainstorage.StorageKindFilesystem,
		StorageID: siID,
		UnitOwner: &internal.StorageInstanceInfoUnitOwner{
			Name: unitName,
			UUID: unitUUID,
		},
		UUID: siUUID,
	})
}

// TestStorageInstanceInfoFilesysteamVolumeBackedUnitAttached tests that
// [State.GetStorageInstanceInfo] correctly returns storage instance information
// for a filesystem-backed storage instance that is also backed by a volume and
// attached to a unit on a machine. The test verifies that the returned info
// includes both filesystem and volume details, mount point, block device
// information with device links, attachment information including machine
// details, unit owner, and status for both filesystem and volume.
func (s *instanceSuite) TestStorageInstanceInfoFilesysteamVolumeBackedUnitAttached(c *tc.C) {
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)
	appUUID, charmUUID := s.newApplication(c, "app1")
	siUUID, siID := s.newFilesystemStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "storage1",
	)
	machineUUID, machineName := s.newMachine(c)
	unitUUID, unitName, unitNetNodeUUID := s.newUnitForApplicationOnMachine(
		c, appUUID, machineUUID,
	)
	s.newStorageInstanceUnitOwner(c, siUUID, unitUUID)
	saUUID := s.newStorageAttachment(c, siUUID, unitUUID)

	fsUUID := s.newModelFilesystem(c, siUUID)
	s.newModelFilesystemAttachmentWithMountPoint(
		c, fsUUID, unitNetNodeUUID, "/mnt/fs1",
	)

	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.setBlockDeviceLinks(
		c, blockDeviceUUID, []string{"/dev/disk/by-id/1", "/dev/disk/123"},
	)
	vUUID := s.newModelVolume(c, siUUID)
	s.newModelVolumeAttachment(
		c, vUUID, unitNetNodeUUID, blockDeviceUUID,
	)

	statusTime := time.Now()
	s.setFilesystemStatus(
		c, fsUUID, domainstatus.StorageFilesystemStatusTypeAttached,
		"testFilesystem", statusTime,
	)
	s.setVolumeStatus(
		c, vUUID, domainstatus.StorageVolumeStatusTypeAttached,
		"testVolume", statusTime,
	)

	st := NewState(s.TxnRunnerFactory())
	info, err := st.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)

	mc := tc.NewMultiChecker()
	// We don't want to assert device name links on ordering.
	mc.AddExpr("_.Attachments[_].Volume.DeviceNameLinks", tc.SameContents, tc.ExpectedValue)
	c.Check(info, mc, internal.StorageInstanceInfo{
		Attachments: []internal.StorageInstanceInfoAttachment{
			{
				Filesystem: &internal.StorageInstanceInfoAttachmentFilesystem{
					MountPoint: "/mnt/fs1",
				},
				Life: domainlife.Alive,
				Machine: &internal.StorageInstanceInfoAttachmentMachine{
					Name: machineName,
					UUID: machineUUID,
				},
				Volume: &internal.StorageInstanceInfoAttachmentVolume{
					DeviceNameLinks: []string{
						"/dev/disk/by-id/1", "/dev/disk/123",
					},
				},
				UnitName: unitName,
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		Filesystem: &internal.StorageInstanceInfoFilesystem{
			Status: &internal.StorageInstanceInfoFilesystemStatus{
				Message:   "testFilesystem",
				Status:    domainstatus.StorageFilesystemStatusTypeAttached,
				UpdatedAt: &statusTime,
			},
			UUID: fsUUID,
		},
		Life:      domainlife.Alive,
		Kind:      domainstorage.StorageKindFilesystem,
		StorageID: siID,
		Volume: &internal.StorageInstanceInfoVolume{
			Status: &internal.StorageInstanceInfoVolumeStatus{
				Message:   "testVolume",
				Status:    domainstatus.StorageVolumeStatusTypeAttached,
				UpdatedAt: &statusTime,
			},
			UUID: vUUID,
		},
		UnitOwner: &internal.StorageInstanceInfoUnitOwner{
			Name: unitName,
			UUID: unitUUID,
		},
		UUID: siUUID,
	})
}

// TestStorageInstanceInfoVolumeUnitAttach tests that
// [State.GetStorageInstanceInfo] correctly returns storage instance information
// for a block storage instance backed by a volume that is attached to a unit on
// a machine. The test verifies that the returned info includes volume details,
// block device information with device links, attachment information including
// machine details, unit owner, and volume status.
func (s *instanceSuite) TestStorageInstanceInfoVolumeUnitAttach(c *tc.C) {
	poolUUID := s.newStoragePool(c, "mypool", "myprovider", nil)
	appUUID, charmUUID := s.newApplication(c, "app1")
	siUUID, siID := s.newBlockStorageInstanceForCharmWithPool(
		c, charmUUID, poolUUID, "storage1",
	)
	machineUUID, machineName := s.newMachine(c)
	unitUUID, unitName, unitNetNodeUUID := s.newUnitForApplicationOnMachine(
		c, appUUID, machineUUID,
	)
	s.newStorageInstanceUnitOwner(c, siUUID, unitUUID)
	saUUID := s.newStorageAttachment(c, siUUID, unitUUID)

	blockDeviceUUID := s.newBlockDevice(c, machineUUID)
	s.setBlockDeviceLinks(c, blockDeviceUUID, []string{"/dev/disk/by-id/123"})
	vUUID := s.newModelVolume(c, siUUID)
	s.newModelVolumeAttachment(
		c, vUUID, unitNetNodeUUID, blockDeviceUUID,
	)

	statusTime := time.Now()
	s.setVolumeStatus(
		c, vUUID, domainstatus.StorageVolumeStatusTypeAttached,
		"testVolume", statusTime,
	)

	st := NewState(s.TxnRunnerFactory())
	info, err := st.GetStorageInstanceInfo(c.Context(), siUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, internal.StorageInstanceInfo{
		Attachments: []internal.StorageInstanceInfoAttachment{
			{
				Life: domainlife.Alive,
				Machine: &internal.StorageInstanceInfoAttachmentMachine{
					Name: machineName,
					UUID: machineUUID,
				},
				Volume: &internal.StorageInstanceInfoAttachmentVolume{
					DeviceNameLinks: []string{
						"/dev/disk/by-id/123",
					},
				},
				UnitName: unitName,
				UnitUUID: unitUUID,
				UUID:     saUUID,
			},
		},
		Life:      domainlife.Alive,
		Kind:      domainstorage.StorageKindBlock,
		StorageID: siID,
		Volume: &internal.StorageInstanceInfoVolume{
			Status: &internal.StorageInstanceInfoVolumeStatus{
				Message:   "testVolume",
				Status:    domainstatus.StorageVolumeStatusTypeAttached,
				UpdatedAt: &statusTime,
			},
			UUID: vUUID,
		},
		UnitOwner: &internal.StorageInstanceInfoUnitOwner{
			Name: unitName,
			UUID: unitUUID,
		},
		UUID: siUUID,
	})
}
