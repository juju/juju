// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	"github.com/juju/juju/domain/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestListStorageInstancesWithEmpty(c *tc.C) {

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIs, nil)
	c.Assert(result, tc.HasLen, 0)
}

func (s *storageSuite) TestListStorageInstancesWithVolume(c *tc.C) {

	ch0 := s.newCharm(c)
	s.newCharmStorage(c, ch0, "blk", storage.StorageKindBlock)

	blkPoolUUID := s.newStoragePool(c, "blkpool", "blkpool", nil)

	statusstate := statusstate.NewModelState(
		s.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	// Volume attachment to a unit with no block device.
	a0 := s.newApplication(c, "foo", ch0)
	nn0 := s.newNetNode(c)
	u0, u0n := s.newUnitWithNetNode(c, a0, nn0)
	s0, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	instanceID0 := s.getStorageID(c, s0)
	s.newStorageAttachment(c, s0, u0)
	v0, _ := s.newVolume(c)
	s.changeVolumeInfo(c, v0, "vol-123", 1234, "hwid", "wwn", true)
	s.newStorageInstanceVolume(c, s0, v0)
	s.newVolumeAttachment(c, v0, nn0)
	s.newStorageUnitOwner(c, s0, u0)
	statusstate.SetVolumeStatus(c.Context(), v0, status.StatusInfo[status.StorageVolumeStatusType]{
		Status:  status.StorageVolumeStatusTypeAttaching,
		Message: "attaching the volumez",
	})

	// Volume attachment to a unit and machine with no block device.
	a1 := s.newApplication(c, "bar", ch0)
	nn1 := s.newNetNode(c)
	_, m1n := s.newMachineWithNetNode(c, nn1)
	u1, u1n := s.newUnitWithNetNode(c, a1, nn1)
	s1, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	instanceID1 := s.getStorageID(c, s1)
	s.newStorageAttachment(c, s1, u1)
	v1, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s1, v1)
	s.newVolumeAttachment(c, v1, nn1)
	s.newStorageUnitOwner(c, s1, u1)

	// Volume attachment to a unit and machine with block device.
	a2 := s.newApplication(c, "baz", ch0)
	nn2 := s.newNetNode(c)
	m2, m2n := s.newMachineWithNetNode(c, nn2)
	u2, u2n := s.newUnitWithNetNode(c, a2, nn2)
	s2, _ := s.newStorageInstance(c, ch0, "blk", blkPoolUUID, storage.StorageKindBlock)
	instanceID2 := s.getStorageID(c, s2)
	s.newStorageAttachment(c, s2, u2)
	v2, _ := s.newVolume(c)
	s.newStorageInstanceVolume(c, s2, v2)
	v2a := s.newVolumeAttachment(c, v2, nn2)
	bd0 := s.newBlockDevice(c, m2, "blocky", "blocky-hw-id", "blocky-bus-addr", []string{
		"/dev/blocky",
		"/dev/disk/by-id/blocky",
	})
	s.changeVolumeAttachmentInfo(c, v2a, bd0, true)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	result, err := st.ListStorageInstances(c.Context())
	c.Assert(err, tc.ErrorIs, nil)
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	c.Assert(result, tc.DeepEquals, []StorageInstanceInfo{
		{
			ID:         instanceID0,
			Owner:      &u0n,
			Kind:       storage.StorageKindBlock,
			Life:       life.Alive,
			Persistent: true,
			VolumeInfo: &VolumeInfo{
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status:  status.StorageVolumeStatusTypeAttaching,
					Message: "attaching the volumez",
				},
				Attachments: []VolumeAttachmentInfo{
					{
						AttachmentInfo: AttachmentInfo{
							Life: life.Alive,
							Unit: u0n,
						},
						HardwareID: "hwid",
						WWN:        "wwn",
					},
				},
			},
		},
		{
			ID:    instanceID1,
			Owner: &u1n,
			Kind:  storage.StorageKindBlock,
			Life:  life.Alive,
			VolumeInfo: &VolumeInfo{
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status: status.StorageVolumeStatusTypePending,
				},
				Attachments: []VolumeAttachmentInfo{
					{
						AttachmentInfo: AttachmentInfo{
							Life:    life.Alive,
							Unit:    u1n,
							Machine: &m1n,
						},
					},
				},
			},
		},
		{
			ID:   instanceID2,
			Kind: storage.StorageKindBlock,
			Life: life.Alive,
			VolumeInfo: &VolumeInfo{
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status: status.StorageVolumeStatusTypePending,
				},
				Attachments: []VolumeAttachmentInfo{
					{
						AttachmentInfo: AttachmentInfo{
							Life:    life.Alive,
							Unit:    u2n,
							Machine: &m2n,
						},
						BlockDeviceName: "blocky",
						BlockDeviceLink: "/dev/blocky",
					},
				},
			},
		},
	})
}
