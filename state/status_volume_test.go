// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type VolumeStatusSuite struct {
	StorageStateSuiteBase
	machine *state.Machine
	volume  state.Volume
}

var _ = gc.Suite(&VolumeStatusSuite{})

func (s *VolumeStatusSuite) SetUpTest(c *gc.C) {
	s.StorageStateSuiteBase.SetUpTest(c)

	machine, err := s.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{{
			Volume: state.VolumeParams{
				Pool: "modelscoped", Size: 1024,
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)

	volume, err := s.storageBackend.Volume(volumeAttachments[0].Volume())
	c.Assert(err, jc.ErrorIsNil)

	s.machine = machine
	s.volume = volume
}

func (s *VolumeStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.volume.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Pending)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *VolumeStatusSuite) TestSetErrorStatusWithoutInfo(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *VolumeStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	validStatuses := []status.Status{
		status.Attaching, status.Attached, status.Detaching,
		status.Detached, status.Destroying,
	}
	for _, status := range validStatuses {
		s.checkGetSetStatus(c, status)
	}
}

func (s *VolumeStatusSuite) checkGetSetStatus(c *gc.C, volumeStatus status.Status) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  volumeStatus,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	volume, err := s.storageBackend.Volume(s.volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := volume.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, volumeStatus)
	c.Check(statusInfo.Message, gc.Equals, "blah")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *VolumeStatusSuite) TestGetSetStatusDying(c *gc.C) {
	err := s.storageBackend.DestroyVolume(s.volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusDead(c *gc.C) {
	err := s.storageBackend.DestroyVolume(s.volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachVolume(s.machine.MachineTag(), s.volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveVolumeAttachment(s.machine.MachineTag(), s.volume.VolumeTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	volume, err := s.storageBackend.Volume(s.volume.VolumeTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volume.Life(), gc.Equals, state.Dead)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c, status.Attaching)
}

func (s *VolumeStatusSuite) TestGetSetStatusGone(c *gc.C) {
	s.obliterateVolume(c, s.volume.VolumeTag())

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "not really",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: volume not found`)

	statusInfo, err := s.volume.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: volume not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *VolumeStatusSuite) TestSetStatusPendingUnprovisioned(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "still",
		Since:   &now,
	}
	err := s.volume.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)
}

func (s *VolumeStatusSuite) TestSetStatusPendingProvisioned(c *gc.C) {
	err := s.storageBackend.SetVolumeInfo(s.volume.VolumeTag(), state.VolumeInfo{
		VolumeId: "vol-ume",
	})
	c.Assert(err, jc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = s.volume.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status "pending"`)
}
