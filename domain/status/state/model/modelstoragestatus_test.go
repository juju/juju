// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see licence file for details.

package model

import (
	"strconv"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type modelStorageStatusSuite struct {
	baseStorageSuite
}

func TestModelStorageStatusSuite(t *testing.T) {
	tc.Run(t, &modelStorageStatusSuite{})
}

// newModelState returns a new ModelState for the suite.
func (s *modelStorageStatusSuite) newModelState(c *tc.C) *ModelState {
	return NewModelState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// newMachineFilesystem inserts a machine-scoped filesystem linked to the
// provided storage instance.
func (s *modelStorageStatusSuite) newMachineFilesystem(
	c *tc.C, siUUID storage.StorageInstanceUUID,
) storage.FilesystemUUID {
	fsUUID := tc.Must(c, storage.NewFilesystemUUID)
	fsID := "machine-fs/" + fsUUID.String()

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
`, fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)
`, siUUID.String(), fsUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID
}

// newMachineVolume creates a new volume in the model with machine provision
// scope and associates it with the provided storage instance. Returned is
// the volume uuid and its volume_id.
func (s *modelStorageStatusSuite) newMachineVolume(
	c *tc.C,
	storageInstanceUUID storage.StorageInstanceUUID,
) (storage.VolumeUUID, string) {
	volumeUUID := tc.Must(c, storage.NewVolumeUUID)
	volumeID := strconv.FormatUint(s.nextSequenceNumber(c, "volume"), 10)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`, volumeUUID.String(), volumeID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
	`, storageInstanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return volumeUUID, volumeID
}

// newDeadModelVolume creates a model-scoped volume in life state Dead and
// associates it with the provided storage instance. Dead storage must be
// excluded from the model status so that the client view agrees with the
// removal guard, which counts only non-dead storage (life_id < 2).
func (s *modelStorageStatusSuite) newDeadModelVolume(
	c *tc.C,
	storageInstanceUUID storage.StorageInstanceUUID,
) string {
	volumeUUID := tc.Must(c, storage.NewVolumeUUID)
	volumeID := strconv.FormatUint(s.nextSequenceNumber(c, "volume"), 10)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id, persistent)
VALUES (?, ?, ?, 0, true)
	`, volumeUUID.String(), volumeID, int(life.Dead))
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)
	`, storageInstanceUUID.String(), volumeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	return volumeID
}

// newOrphanModelFilesystem inserts a model-scoped filesystem with no
// storage-instance link. The removal guard counts such rows directly from
// storage_filesystem, so the model status must report them too.
func (s *modelStorageStatusSuite) newOrphanModelFilesystem(
	c *tc.C,
) (storage.FilesystemUUID, string) {
	fsUUID := tc.Must(c, storage.NewFilesystemUUID)
	fsID := "orphan-fs/" + fsUUID.String()

	_, err := s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
`, fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
}

// newOrphanModelVolume inserts a model-scoped volume with no
// storage-instance link. The removal guard counts such rows directly from
// storage_volume, so the model status must report them too.
func (s *modelStorageStatusSuite) newOrphanModelVolume(c *tc.C) (storage.VolumeUUID, string) {
	volumeUUID := tc.Must(c, storage.NewVolumeUUID)
	volumeID := strconv.FormatUint(s.nextSequenceNumber(c, "volume"), 10)

	_, err := s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id, persistent)
VALUES (?, ?, 0, 0, true)
`, volumeUUID.String(), volumeID)
	c.Assert(err, tc.ErrorIsNil)

	return volumeUUID, volumeID
}

// TestGetModelStorageStatusesEmpty asserts that a model without storage
// returns empty filesystem and volume lists.
func (s *modelStorageStatusSuite) TestGetModelStorageStatusesEmpty(c *tc.C) {
	st := s.newModelState(c)

	statuses, err := st.GetModelStorageStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statuses.Filesystems, tc.HasLen, 0)
	c.Check(statuses.Volumes, tc.HasLen, 0)
}

// TestGetModelStorageStatuses asserts that filesystems and volumes are
// reported with their provider id, status and detachability, where
// detachability follows the model-scope rule with volume-first precedence
// for volume-backed filesystems, and that dead storage is not reported.
func (s *modelStorageStatusSuite) TestGetModelStorageStatuses(c *tc.C) {
	poolUUID := s.newStoragePool(c, "kubernetes", "kubernetes", nil)
	charmUUID := s.newCharm(c)

	// 1. A model-scoped volume-less filesystem (e.g. an imported k8s PV):
	//    detachable. Its filesystem_id is "foo/<fs-uuid>" per the helper.
	modelFSInstance, _ := s.newStorageInstance(
		c, charmUUID, "model-fs", poolUUID, storage.StorageKindFilesystem,
	)
	modelFSUUID, _ := s.newFilesystem(c)
	s.newStorageInstanceFilesystem(c, modelFSInstance, modelFSUUID)

	// 2. A machine-scoped volume-less filesystem (e.g. rootfs): not
	//    detachable.
	machineFSInstance, _ := s.newStorageInstance(
		c, charmUUID, "machine-fs", poolUUID, storage.StorageKindFilesystem,
	)
	machineFSUUID := s.newMachineFilesystem(c, machineFSInstance)

	// 3. A model-scoped volume: detachable.
	volumeInstance, _ := s.newStorageInstance(
		c, charmUUID, "model-vol", poolUUID, storage.StorageKindBlock,
	)
	modelVolumeUUID, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, volumeInstance, modelVolumeUUID)

	// 4. A machine-scoped filesystem backed by a model-scoped volume:
	//    detachable via volume-first precedence.
	volBackedFSInstance, _ := s.newStorageInstance(
		c, charmUUID, "vol-backed-fs", poolUUID, storage.StorageKindFilesystem,
	)
	volBackedFSUUID := s.newMachineFilesystem(c, volBackedFSInstance)
	volBackedVolumeUUID, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, volBackedFSInstance, volBackedVolumeUUID)

	// 5. A machine-scoped volume (e.g. loop/root disk): not detachable.
	machineVolInstance, _ := s.newStorageInstance(
		c, charmUUID, "machine-vol", poolUUID, storage.StorageKindBlock,
	)
	_, machineVolID := s.newMachineVolume(c, machineVolInstance)

	// 6. A dead model-scoped volume: excluded entirely, so the model
	//    status agrees with the removal guard's non-dead filter.
	deadVolInstance, _ := s.newStorageInstance(
		c, charmUUID, "dead-vol", poolUUID, storage.StorageKindBlock,
	)
	deadVolID := s.newDeadModelVolume(c, deadVolInstance)

	st := s.newModelState(c)
	statuses, err := st.GetModelStorageStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(statuses.Filesystems, tc.HasLen, 3)
	c.Check(statuses.Volumes, tc.HasLen, 3)

	detachableByID := make(map[string]bool)
	for _, fs := range statuses.Filesystems {
		detachableByID[fs.ID] = fs.Detachable
	}
	modelFSDetachable, ok := detachableByID["foo/"+modelFSUUID.String()]
	c.Assert(ok, tc.IsTrue)
	c.Check(modelFSDetachable, tc.IsTrue)
	machineFSDetachable, ok := detachableByID["machine-fs/"+machineFSUUID.String()]
	c.Assert(ok, tc.IsTrue)
	c.Check(machineFSDetachable, tc.IsFalse)
	// Machine-scoped filesystem backed by a model-scoped volume.
	volBackedFSDetachable, ok := detachableByID["machine-fs/"+volBackedFSUUID.String()]
	c.Assert(ok, tc.IsTrue)
	c.Check(volBackedFSDetachable, tc.IsTrue)

	// Exactly two detachable (model-scoped) volumes, the machine-scoped
	// one is not detachable, and the dead volume is not reported.
	detachableVolumeCount := 0
	machineVolumeFound := false
	for _, vol := range statuses.Volumes {
		c.Check(vol.ID, tc.Not(tc.Equals), deadVolID)
		if vol.ID == machineVolID {
			machineVolumeFound = true
			c.Check(vol.Detachable, tc.IsFalse)
			continue
		}
		c.Check(vol.Detachable, tc.IsTrue)
		detachableVolumeCount++
	}
	c.Check(machineVolumeFound, tc.IsTrue)
	c.Check(detachableVolumeCount, tc.Equals, 2)
}

// TestGetModelStorageStatusesOrphanStorage asserts that model-scoped
// filesystems and volumes with no storage-instance link are still
// reported. The removal cascade's persistent-storage guard counts those
// rows directly from storage_filesystem/storage_volume and refuses model
// teardown because of them, so the client view must see them too: if it
// did not, destroy-controller would refuse server-side while the model
// status showed no storage at all.
func (s *modelStorageStatusSuite) TestGetModelStorageStatusesOrphanStorage(c *tc.C) {
	_, orphanFSID := s.newOrphanModelFilesystem(c)
	_, orphanVolumeID := s.newOrphanModelVolume(c)

	st := s.newModelState(c)
	statuses, err := st.GetModelStorageStatuses(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Both orphans are reported, and both are detachable: they are
	// model-scoped, exactly what the guard refuses on.
	c.Check(statuses.Filesystems, tc.DeepEquals, []status.ModelStorageFilesystemStatus{{
		ID:         orphanFSID,
		Detachable: true,
	}})
	c.Check(statuses.Volumes, tc.DeepEquals, []status.ModelStorageVolumeStatus{{
		ID:         orphanVolumeID,
		Detachable: true,
	}})
}
