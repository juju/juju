// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/application"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
)

// filesystemSuite provides a set of tests for asserting the state interface
// for filesystems in the model.
type filesystemSuite struct {
	baseSuite
}

// TestFilesystemSuite runs the tests in [filesystemSuite].
func TestFilesystemSuite(t *testing.T) {
	tc.Run(t, &filesystemSuite{})
}

// TestCheckFilesystemForIDExists tests the happy path of
// [State.CheckFilesystemForIDExists].
func (s *filesystemSuite) TestCheckFilesystemForIDExists(c *tc.C) {
	_, id := s.newModelFilesystem(c)
	st := NewState(s.TxnRunnerFactory())

	exists, err := st.CheckFilesystemForIDExists(c.Context(), id)

	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *filesystemSuite) TestCheckFilesystemForIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	exists, err := st.CheckFilesystemForIDExists(c.Context(), "no-exist")

	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

// TestGetFilesystemWithBackingVolume tests getting a filesystem's information
// by id when it is backed by a volume.
func (s *filesystemSuite) TestGetFilesystemWithBackingVolume(c *tc.C) {
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "filesystem", false, "")
	poolUUID := s.newStoragePool(c, "rootfs", "rootfs", nil)
	storageInstanceUUID := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	volUUID, volID := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, storageInstanceUUID, volUUID)
	fsUUID, fsID := s.newMachineFilesystemWithSize(c, 100)
	s.setFilesystemProviderID(c, fsUUID, "fs-123")
	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	st := NewState(s.TxnRunnerFactory())

	result, err := st.GetFilesystem(c.Context(), fsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, storageprovisioning.Filesystem{
		BackingVolume: &storageprovisioning.FilesystemBackingVolume{
			VolumeID: volID,
		},
		FilesystemID: fsID,
		ProviderID:   "fs-123",
		SizeMiB:      100,
	})
}

// TestGetFilesystemWithBackingVolume tests getting a filesystem's information
// by id when it isn't backed by a volume.
func (s *filesystemSuite) TestGetFilesystemWithoutBackingVolume(c *tc.C) {
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "filesystem", false, "")
	poolUUID := s.newStoragePool(c, "rootfs", "rootfs", nil)
	storageInstanceUUID := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	fsUUID, fsID := s.newMachineFilesystemWithSize(c, 100)
	s.setFilesystemProviderID(c, fsUUID, "fs-123")
	s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	st := NewState(s.TxnRunnerFactory())

	result, err := st.GetFilesystem(c.Context(), fsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, storageprovisioning.Filesystem{
		FilesystemID: fsID,
		ProviderID:   "fs-123",
		SizeMiB:      100,
	})
}

func (s *filesystemSuite) TestGetFilesystemNotFoundError(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	notFoundUUID := domaintesting.GenFilesystemUUID(c)

	_, err := st.GetFilesystem(c.Context(), notFoundUUID)

	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

// TestGetFilesystemNotAttachedToStorageInstance is a regression test to show
// that when calling [State.GetFilesystem] for a filesystem uuid that is not
// associated with a storage instance, the filesystem is correctly returned
// and doesn't result in a [storageprovisioningerrors.FilesystemNotFound] error.
//
// This bug was found because part of the filesystem information tries to
// establish a back volume for the filesystem. This is done via linking through
// the storage instance. It is not an error if this information doesn't exist.
// It just means that the filesystem is not backed by a volume in the model.
func (s *filesystemSuite) TestGetFilesystemNotAttachedToStorageInstance(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	fsUUID, fsID := s.newMachineFilesystemWithSize(c, 100)
	s.setFilesystemProviderID(c, fsUUID, "fs-123")

	fs, err := st.GetFilesystem(c.Context(), fsUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(fs.FilesystemID, tc.Equals, fsID)
	c.Check(fs.ProviderID, tc.Equals, "fs-123")
	c.Check(fs.SizeMiB, tc.Equals, uint64(100))
}

func (s *filesystemSuite) TestGetFilesystemAttachment(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	fsUUID, fsID := s.newMachineFilesystem(c)
	uuid := s.newMachineFilesystemAttachmentWithMount(
		c, fsUUID, netNodeUUID, "/mnt/", true,
	)

	result, err := st.GetFilesystemAttachment(c.Context(), uuid)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, storageprovisioning.FilesystemAttachment{
		FilesystemID: fsID,
		MountPoint:   "/mnt/",
		ReadOnly:     true,
	})
}

func (s *filesystemSuite) TestGetFilesystemAttachmentNotFound(c *tc.C) {
	notFoundUUID := domaintesting.GenFilesystemAttachmentUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemAttachment(
		c.Context(), notFoundUUID,
	)

	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

// TestGetFilesystemTemplatesForApplication checks that multiple storage for
// different pools return different values.
func (s *filesystemSuite) TestGetFilesystemTemplatesForApplication(c *tc.C) {
	appUUID, charmUUID := s.newApplication(c, "foo")

	spUUID := s.newStoragePool(c, "water", "magic", map[string]string{
		"a": "b",
		"c": "d",
	})
	spUUID2 := s.newStoragePool(c, "rootfs", "rootfs", nil)
	s.newCharmStorage(c, charmUUID, "x", "filesystem", true, "/a/x")
	s.newCharmStorage(c, charmUUID, "y", "filesystem", true, "/a/y")
	s.newApplicationStorageDirective(c, appUUID, charmUUID, "x", spUUID, 123, 2)
	s.newApplicationStorageDirective(c, appUUID, charmUUID, "y", spUUID2, 456, 1)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemTemplatesForApplication(c.Context(), application.ID(appUUID))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []storageprovisioning.FilesystemTemplate{{
		StorageName:  "x",
		Count:        2,
		MaxCount:     10,
		SizeMiB:      123,
		ProviderType: "magic",
		ReadOnly:     true,
		Location:     "/a/x",
		Attributes: map[string]string{
			"a": "b",
			"c": "d",
		},
	}, {
		StorageName:  "y",
		Count:        1,
		MaxCount:     10,
		SizeMiB:      456,
		ProviderType: "rootfs",
		ReadOnly:     true,
		Location:     "/a/y",
	}})
}

// TestGetFilesystemAttachmentIDsOnlyUnits tests that when requesting ids for a
// filesystem attachment and no machines are using the net node the unit name is
// reported.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsOnlyUnits(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID.String(): {
			FilesystemID: fsID,
			MachineName:  nil,
			UnitName:     &unitName,
		},
	})
}

// TestGetFilesystemAttachmentIDsOnlyMachines tests that when requesting ids for a
// filesystem attachment and the net node is attached to a machine the machine
// name is set.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsOnlyMachines(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID.String(): {
			FilesystemID: fsID,
			MachineName:  &machineName,
			UnitName:     nil,
		},
	})
}

// TestGetFilesystemAttachmentIDsMachineNotUnit tests that when requesting ids for a
// filesystem attachment and the net node is attached to a machine the machine
// name is set. This should remain true when the net node is also used by a
// unit. This is a valid case when units are assigned to a machine.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsMachineNotUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID.String(): {
			FilesystemID: fsID,
			MachineName:  &machineName,
			UnitName:     nil,
		},
	})
}

// TestGetFilesystemAttachmentIDsMixed tests that when requesting ids for a
// mixed set of filesystem attachments uuids the machine name and unit name are
// correctly set.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsMixed(c *tc.C) {
	netNodeUUID1 := s.newNetNode(c)
	netNodeUUID2 := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID1)
	appUUID, _ := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID2)

	fs1UUID, fsID1 := s.newMachineFilesystem(c)
	fsa1UUID := s.newMachineFilesystemAttachment(c, fs1UUID, netNodeUUID1)

	fs2UUID, fsID2 := s.newMachineFilesystem(c)
	fsa2UUID := s.newMachineFilesystemAttachment(c, fs2UUID, netNodeUUID2)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{
		fsa1UUID.String(), fsa2UUID.String(),
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsa1UUID.String(): {
			FilesystemID: fsID1,
			MachineName:  &machineName,
			UnitName:     nil,
		},
		fsa2UUID.String(): {
			FilesystemID: fsID2,
			MachineName:  nil,
			UnitName:     &unitName,
		},
	})
}

// TestGetFilesystemAttachmentIDsNotMachineOrUnit tests that when requesting
// ids for a filesystem attachment that is using a net node not attached to a
// machine or unit the uuid is dropped from the final result.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsNotMachineOrUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID, _ := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetFilesystemAttachmentIDsNotFound tests that when requesting ids for
// filesystem attachment uuids that don't exist the uuids are excluded from the
// result with no error returned.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{"no-exist"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetFilesystemAttachmentLifeForNetNode tests that the correct life is
// reported for each model provisioned filesystem attachment associated with the
// given net node.
//
// We also inject a life change during the test to make sure that it is
// reflected.
func (s *filesystemSuite) TestGetFilesystemAttachmentLifeForNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID1, _ := s.newMachineFilesystem(c)
	fsUUID2, _ := s.newMachineFilesystem(c)
	fsUUID3, _ := s.newMachineFilesystem(c)
	fsaUUID1 := s.newMachineFilesystemAttachment(c, fsUUID1, netNodeUUID)
	fsaUUID2 := s.newMachineFilesystemAttachment(c, fsUUID2, netNodeUUID)
	fsaUUID3 := s.newMachineFilesystemAttachment(c, fsUUID3, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		fsaUUID1.String(): domainlife.Alive,
		fsaUUID2.String(): domainlife.Alive,
		fsaUUID3.String(): domainlife.Alive,
	})

	// Apply a life change to one of the attachments and check the change comes
	// out.
	s.changeFilesystemAttachmentLife(c, fsaUUID1, domainlife.Dying)
	lives, err = st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		fsaUUID1.String(): domainlife.Dying,
		fsaUUID2.String(): domainlife.Alive,
		fsaUUID3.String(): domainlife.Alive,
	})
}

// TestGetFilesystemAttachmentLifeNoResults tests that when no attachment lives
// exist for a net node an empty result is returned with no error.
func (s *filesystemSuite) TestGetFilesystemAttachmentLifeNoResults(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

// TestGetFilesystemLifeForNetNode tests if we can get the filesystem life for
// filesystems attached to a specified machine's net node.
func (s *filesystemSuite) TestGetFilesystemLifeForNetNode(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	fsOneUUID, fsOneID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, fsTwoID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)
	fsThreeUUID, fsThreeID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsThreeUUID, netNodeUUID)

	s.changeFilesystemLife(c, fsTwoUUID, domainlife.Dying)
	s.changeFilesystemLife(c, fsThreeUUID, domainlife.Dead)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	fsUUIDs, err := st.GetFilesystemLifeForNetNode(
		c.Context(), netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsOneID:   domainlife.Alive,
		fsTwoID:   domainlife.Dying,
		fsThreeID: domainlife.Dead,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystems tests the initial query
// for machine provisioned filesystems watcher returns only the filesystem UUIDs
// attached to the specified machine net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystems(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	fsOneUUID, fsOneID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, fsTwoID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	fsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsOneID: domainlife.Alive,
		fsTwoID: domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystemsNone tests the initial
// query for machine provisioned filesystems watcher does not return an error
// when no machine provisioned filesystems are attached to the specified machine
// net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemsNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	fsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedFilesystemsNetNodeMissing tests
// the initial query for machine provisioned filesystems watcher errors when the
// net node specified is not found.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemsNetNodeMissing(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedFilesystemsNone tests the initial
// query for a model provisioned filsystem watcher returns no error when there
// is not any model provisioned filesystems.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemsNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, _ = s.newMachineFilesystem(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystems()
	c.Check(ns, tc.Equals, "storage_filesystem_life_model_provisioning")

	db := s.TxnRunner()
	fsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedFilesystems tests the initial query
// for a model provisioned filsystem watcher returns only the filesystem IDs for
// the model provisoned filesystems.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystems(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, fsOneID := s.newModelFilesystem(c)
	_, fsTwoID := s.newModelFilesystem(c)
	_, _ = s.newMachineFilesystem(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystems()
	c.Check(ns, tc.Equals, "storage_filesystem_life_model_provisioning")

	db := s.TxnRunner()
	fsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsIDs, tc.SameContents, []string{fsOneID, fsTwoID})
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachments tests the
// initial query for machine provisioned filesystem attachments watcher returns
// only the filesystem attachment UUIDs attached to the specified net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsOneUUID, _ := s.newMachineFilesystem(c)
	fsaOneUUID := s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, _ := s.newMachineFilesystem(c)
	fsaTwoUUID := s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsaTwoUUID.String(): domainlife.Alive,
		fsaOneUUID.String(): domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNone tests
// the initial query for machine provisioned filesystem attachments watcher does
// not return an error when no machine provisioned filesystem attachments are
// attached to the specified net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNone(c *tc.C) {
	netNodeUUID := s.newNetNode(c)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNetNodeMissing
// tests the initial query for machine provisioned filesystem attachmewnts
// watcher errors when the net node specified is not found.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNetNodeMissing(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	// We don't focus on what the error is as no specific error type is offered
	// as part of the contract. We just care that an error occurred.
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedFilesystemAttachmentsNone tests the
// initial query for a model provisioned filsystem attachment watcher returns no
// error when there is no model provisioned filesystem attachments.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemAttachmentsNone(c *tc.C) {
	// Create a machine based filesystem attachment to assert  this doesn't show
	// up.
	netNode := s.newNetNode(c)
	fsUUID, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsUUID, netNode)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_model_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedFilesystemAttachments tests the
// initial query for a model provisioned filsystem attachment watcher returns
// only the filesystem attachment uuids for the model provisoned filesystem
// attachments.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	fsOneUUID, _ := s.newModelFilesystem(c)
	fsTwoUUID, _ := s.newModelFilesystem(c)
	fsThreeUUID, _ := s.newMachineFilesystem(c)
	fsaOneUUID := s.newModelFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsaTwoUUID := s.newModelFilesystemAttachment(c, fsTwoUUID, netNodeUUID)
	s.newMachineFilesystemAttachment(c, fsThreeUUID, netNodeUUID)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_model_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.SameContents, []string{
		fsaOneUUID.String(), fsaTwoUUID.String(),
	})
}

// TestGetFilesystemAttachmentLife tests that asking for the life of a filesystem
// attachment that doesn't exist returns to the caller an error satisfying
// [storageprovisioningerrors.FilesystemAttachmentNotFound].
func (s *filesystemSuite) TestGetFilesystemAttachmentLifeNotFound(c *tc.C) {
	uuid := domaintesting.GenFilesystemAttachmentUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentLife(c *tc.C) {
	fsUUID, _ := s.newModelFilesystem(c)
	netNodeUUID := s.newNetNode(c)
	uuid := s.newModelFilesystemAttachment(c, fsUUID, netNodeUUID)
	st := NewState(s.TxnRunnerFactory())

	life, err := st.GetFilesystemAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Alive)

	// Update the life and confirm that it is reflected out again.
	s.changeFilesystemAttachmentLife(c, uuid, domainlife.Dying)
	life, err = st.GetFilesystemAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Dying)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentUUIDForIDNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID, _ := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		c.Context(), fsUUID, netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid.String(), tc.Equals, fsaUUID.String())
}

// TestGetFilesystemAttachmentUUIDForIDNetNodeFSNotFound tests that the caller
// get backs a [storageprovisioningerrors.FilesystemNotFound] error when asking
// for an attachment using a filesystem uuid that does not exist in the model.
func (s *filesystemSuite) TestGetFilesystemAttachmentUUIDForIDNetNodeFSNotFound(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	notFoundFS := domaintesting.GenFilesystemUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		c.Context(), notFoundFS, netNodeUUID,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

// TestGetFilesystemAttachmentUUIDForIDNetNodeNetNodeNotFound tests that the
// caller get backs a [networkerrors.NetNodeNotFound] error when asking
// for an attachment using a net node uuid that does not exist in the model.
func (s *filesystemSuite) TestGetFilesystemAttachmentUUIDForIDNetNodeNetNodeNotFound(c *tc.C) {
	notFoundNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	fsUUID, _ := s.newModelFilesystem(c)
	st := NewState(s.TxnRunnerFactory())

	_, err = st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		c.Context(), fsUUID, notFoundNodeUUID,
	)

	c.Check(err, tc.ErrorIs, networkerrors.NetNodeNotFound)
}

// TestGetFilesystemAttachmentUUIDForIDNetNodeUnrelated tests that if the
// filesystem uuid and net node uuid exist but are unrelated within an
// attachment an error satisfying
// [storageprovisioningerrors.FilesystemAttachmentNotFound] is returned.
func (s *filesystemSuite) TestGetFilesystemAttachmentUUIDForIDNetNodeUnrelated(c *tc.C) {
	nnUUIDOne := s.newNetNode(c)
	nnUUIDTwo := s.newNetNode(c)
	fsUUIDOne, _ := s.newMachineFilesystem(c)
	fsUUIDTwo, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsUUIDOne, nnUUIDOne)
	s.newMachineFilesystemAttachment(c, fsUUIDTwo, nnUUIDTwo)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemAttachmentUUIDForFilesystemNetNode(
		c.Context(), fsUUIDOne, nnUUIDTwo,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

// TestGetFilesystemLifeNotFound tests that asking for the life of a filesystem
// attachment that doesn't exist returns to the caller an error satisfying
// [storageprovisioningerrors.FilesystemNotFound].
func (s *filesystemSuite) TestGetFilesystemLifeNotFound(c *tc.C) {
	uuid := domaintesting.GenFilesystemUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *filesystemSuite) TestGetFilesystemLife(c *tc.C) {
	fsUUID, _ := s.newModelFilesystem(c)
	st := NewState(s.TxnRunnerFactory())

	life, err := st.GetFilesystemLife(c.Context(), fsUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Alive)

	// Update the life and confirm that it is reflected out again.
	s.changeFilesystemLife(c, fsUUID, domainlife.Dying)
	life, err = st.GetFilesystemLife(c.Context(), fsUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Dying)
}

// TestGetFilesystemUUIDForIDNotFound tests that asking for the uuid of a
// filesystem using an id that does not exist returns an error satisfying
// [storageprovisioningerrors.FilesystemNotFound] to the caller.
func (s *filesystemSuite) TestGetFilesystemUUIDForIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetFilesystemUUIDForID(c.Context(), "no-exist")
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *filesystemSuite) TestGetFilesystemUUIDForID(c *tc.C) {
	fsUUID, fsID := s.newModelFilesystem(c)
	st := NewState(s.TxnRunnerFactory())

	gotUUID, err := st.GetFilesystemUUIDForID(c.Context(), fsID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID.String(), tc.Equals, fsUUID.String())
}

// TestSetFilesystemProvisionedInfo checks if SetFilesystemProvisionedInfo only
// affects the specified filesystem.
func (s *filesystemSuite) TestSetFilesystemProvisionedInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	fsUUID, _ := s.newModelFilesystem(c)
	fsOtherUUID, _ := s.newModelFilesystem(c)

	info := storageprovisioning.FilesystemProvisionedInfo{
		ProviderID: "xyz",
		SizeMiB:    123,
	}
	err := st.SetFilesystemProvisionedInfo(c.Context(), fsUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	fs, err := st.GetFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fs.ProviderID, tc.Equals, "xyz")
	c.Check(fs.SizeMiB, tc.Equals, uint64(123))

	otherFs, err := st.GetFilesystem(c.Context(), fsOtherUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(otherFs.ProviderID, tc.Not(tc.Equals), "xyz")
	c.Check(otherFs.SizeMiB, tc.Not(tc.Equals), uint64(123))
}

// TestSetFilesystemAttachmentProvisionedInfo checks that a call to
// SetFilesystemAttachmentProvisionedInfo sets the info on the specified
// filesystem attachment.
func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	fsUUID, _ := s.newModelFilesystem(c)
	netNodeUUID := s.newNetNode(c)
	s.newMachineWithNetNode(c, netNodeUUID)
	fsAttachmentUUID := s.newModelFilesystemAttachment(c, fsUUID, netNodeUUID)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x/y/z",
		ReadOnly:   true,
	}
	err := st.SetFilesystemAttachmentProvisionedInfo(c.Context(),
		fsAttachmentUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	fsAttachment, err := st.GetFilesystemAttachment(c.Context(),
		fsAttachmentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsAttachment.MountPoint, tc.Equals, "x/y/z")
	c.Check(fsAttachment.ReadOnly, tc.IsTrue)
}

// TestSetFilesystemAttachmentProvisionedInfoNotFound checks that a call to
// SetFilesystemAttachmentProvisionedInfo where no filesystem attachent exists
// for the supplied machine and filesystem, an error is returned.
func (s *filesystemSuite) TestSetFilesystemAttachmentProvisionedInfoNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid, err := storageprovisioning.NewFilesystemAttachmentUUID()
	c.Assert(err, tc.ErrorIsNil)

	info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
		MountPoint: "x/y/z",
		ReadOnly:   true,
	}
	err = st.SetFilesystemAttachmentProvisionedInfo(c.Context(), uuid, info)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.FilesystemAttachmentNotFound)
}

// TestGetFilesystemParamsNotFound checks that when asking for filesystem params
// and the filesystem doesn't exist the caller gets back an error satisfying
// [storageprovisioningerrors.FilesystemNotFound].
func (s *filesystemSuite) TestGetFilesystemParamsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	fsUUID := domaintesting.GenFilesystemUUID(c)

	_, err := st.GetFilesystemParams(c.Context(), fsUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

// TestGetFilesystemParamsUsingPool is testing getting filesystem params
// where the associated storage instance is referencing a storage pool.
func (s *filesystemSuite) TestGetFilesystemParamsUsingPool(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	poolUUID := s.newStoragePool(c, "mypool", "mypoolprovider", map[string]string{
		"foo": "bar",
	})
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "filesystem", false, "")
	suuid := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	fsUUID, fsID := s.newMachineFilesystemWithSize(c, 100)
	s.newStorageInstanceFilesystem(c, suuid, fsUUID)

	params, err := st.GetFilesystemParams(c.Context(), fsUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.FilesystemParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       fsID,
		Provider: "mypoolprovider",
		SizeMiB:  100,
	})
}

// TestGetFilesystemAttachmentParamsNotFound is testing that when asking for the
// params of a filesystem attachment that does not exist the caller gets back an
// error satisfying [storageprovisioningerrors.FilesystemAttachmentNotFound].
func (s *filesystemSuite) TestGetFilesystemAttachmentParamsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	fsaUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	_, err := st.GetFilesystemAttachmentParams(c.Context(), fsaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *filesystemSuite) TestGetFilesystemAttachmentParamsUsingPoolAndMachine(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	netNodeUUID := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNodeUUID)
	s.newMachineCloudInstanceWithID(c, machineUUID, "machine-id-123")
	poolUUID := s.newStoragePool(c, "thebigpool", "canonical", map[string]string{
		"foo": "bar",
	})
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "filesystem", true, "/var/foo")
	suuid := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	fsUUID, _ := s.newMachineFilesystemWithSize(c, 100)
	s.setFilesystemProviderID(c, fsUUID, "provider-id")
	fsaUUID := s.newMachineFilesystemAttachmentWithMount(c, fsUUID, netNodeUUID, "/var/foo", true)
	s.newStorageInstanceFilesystem(c, suuid, fsUUID)

	params, err := st.GetFilesystemAttachmentParams(c.Context(), fsaUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.FilesystemAttachmentParams{
		MachineInstanceID: "machine-id-123",
		Provider:          "canonical",
		ProviderID:        "provider-id",
		MountPoint:        "/var/foo",
		ReadOnly:          true,
	})
}

func (s *filesystemSuite) TestGetFilesystemAttachmentParamsUsingPoolAndUnit(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	netNodeUUID := s.newNetNode(c)
	appUUID, charmUUID := s.newApplication(c, "myapp")
	s.newUnitWithNetNode(c, "myapp/0", appUUID, netNodeUUID)
	poolUUID := s.newStoragePool(c, "mybigstoragepool", "poolprovider", nil)
	s.newCharmStorage(c, charmUUID, "mystorage", "filesystem", true, "/var/foo")
	suuid := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	fsUUID, _ := s.newModelFilesystem(c)
	s.setFilesystemProviderID(c, fsUUID, "provider-id")
	fsaUUID := s.newModelFilesystemAttachmentWithMount(c, fsUUID, netNodeUUID, "", false)
	s.newStorageInstanceFilesystem(c, suuid, fsUUID)

	params, err := st.GetFilesystemAttachmentParams(c.Context(), fsaUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, storageprovisioning.FilesystemAttachmentParams{
		MachineInstanceID: "",
		Provider:          "poolprovider",
		ProviderID:        "provider-id",
		MountPoint:        "/var/foo",
		ReadOnly:          true,
	})
}

// changeFilesystemLife is a utility function for updating the life value of a
// filesystem.
func (s *filesystemSuite) changeFilesystemLife(
	c *tc.C, uuid storageprovisioning.FilesystemUUID, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid.String())
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentLife is a utility function for updating the life
// value of a filesystem attachment. This is used to trigger an update trigger
// for a filesystem attachment.
func (s *filesystemSuite) changeFilesystemAttachmentLife(
	c *tc.C,
	uuid storageprovisioning.FilesystemAttachmentUUID,
	life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid.String())
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *filesystemSuite) newMachineFilesystem(c *tc.C) (
	storageprovisioning.FilesystemUUID, string,
) {
	return s.newMachineFilesystemWithSize(c, 100)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope and the supplied size. Returned is the uuid and filesystem
// id of the entity.
func (s *filesystemSuite) newMachineFilesystemWithSize(
	c *tc.C, size uint64,
) (storageprovisioning.FilesystemUUID, string) {
	fsUUID := domaintesting.GenFilesystemUUID(c)
	fsID := fmt.Sprintf("foo/%s", fsUUID.String())
	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, size_mib, provision_scope_id)
VALUES (?, ?, 0, ?, 1)
	`,
		fsUUID.String(), fsID, size)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Return is the uuid and filesystem id of the entity.
func (s *filesystemSuite) newModelFilesystem(c *tc.C) (
	storageprovisioning.FilesystemUUID, string,
) {
	fsUUID := domaintesting.GenFilesystemUUID(c)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

// newMachineFilesystemAttachment creates a new filesystem attachment that has
// machine provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *filesystemSuite) newMachineFilesystemAttachment(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.FilesystemAttachmentUUID {
	return s.newMachineFilesystemAttachmentWithMount(
		c, fsUUID, netNodeUUID, "", false,
	)
}

func (s *filesystemSuite) newMachineFilesystemAttachmentWithMount(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	mountPoint string,
	readOnly bool,
) storageprovisioning.FilesystemAttachmentUUID {
	attachmentUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           mount_point,
                                           read_only,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, ?, ?, 1)
`,
		attachmentUUID.String(),
		fsUUID.String(),
		netNodeUUID.String(),
		mountPoint,
		readOnly,
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

// newModelFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *filesystemSuite) newModelFilesystemAttachment(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
) storageprovisioning.FilesystemAttachmentUUID {
	return s.newModelFilesystemAttachmentWithMount(
		c, fsUUID, netNodeUUID, "/mnt", false,
	)
}

func (s *filesystemSuite) newModelFilesystemAttachmentWithMount(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	netNodeUUID domainnetwork.NetNodeUUID,
	mountPoint string,
	readOnly bool,
) storageprovisioning.FilesystemAttachmentUUID {
	attachmentUUID := domaintesting.GenFilesystemAttachmentUUID(c)

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           mount_point,
                                           read_only,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, ?, ?, 0)
`,
		attachmentUUID.String(),
		fsUUID,
		netNodeUUID.String(),
		mountPoint,
		readOnly,
	)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID
}

func (s *filesystemSuite) setFilesystemProviderID(
	c *tc.C,
	fsUUID storageprovisioning.FilesystemUUID,
	providerID string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    provider_id = ?
WHERE  uuid = ?
`,
		providerID, fsUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}
