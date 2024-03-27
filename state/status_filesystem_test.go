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

type FilesystemStatusSuite struct {
	StorageStateSuiteBase
	machine    *state.Machine
	filesystem state.Filesystem
}

var _ = gc.Suite(&FilesystemStatusSuite{})

func (s *FilesystemStatusSuite) SetUpTest(c *gc.C) {
	s.StorageStateSuiteBase.SetUpTest(c)

	machine, err := s.State.AddOneMachine(defaultInstancePrechecker, state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Filesystems: []state.HostFilesystemParams{{
			Filesystem: state.FilesystemParams{
				Pool: "modelscoped", Size: 1024,
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	filesystemAttachments, err := s.storageBackend.MachineFilesystemAttachments(machine.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)

	filesystem, err := s.storageBackend.Filesystem(filesystemAttachments[0].Filesystem())
	c.Assert(err, jc.ErrorIsNil)

	s.machine = machine
	s.filesystem = filesystem
}

func (s *FilesystemStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.filesystem.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Pending)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *FilesystemStatusSuite) TestSetErrorStatusWithoutInfo(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *FilesystemStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) checkGetSetStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	filesystem, err := s.storageBackend.Filesystem(s.filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := filesystem.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Attaching)
	c.Check(statusInfo.Message, gc.Equals, "blah")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *FilesystemStatusSuite) TestGetSetStatusDying(c *gc.C) {
	err := s.storageBackend.DestroyFilesystem(s.filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusDead(c *gc.C) {
	err := s.storageBackend.DestroyFilesystem(s.filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.DetachFilesystem(s.machine.MachineTag(), s.filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.storageBackend.RemoveFilesystemAttachment(s.machine.MachineTag(), s.filesystem.FilesystemTag(), false)
	c.Assert(err, jc.ErrorIsNil)

	filesystem, err := s.storageBackend.Filesystem(s.filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.Life(), gc.Equals, state.Dead)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *FilesystemStatusSuite) TestGetSetStatusGone(c *gc.C) {
	s.obliterateFilesystem(c, s.filesystem.FilesystemTag())

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Attaching,
		Message: "not really",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status: filesystem not found`)

	statusInfo, err := s.filesystem.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: filesystem not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *FilesystemStatusSuite) TestSetStatusPendingUnprovisioned(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "still",
		Since:   &now,
	}
	err := s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)
}

func (s *FilesystemStatusSuite) TestSetStatusPendingProvisioned(c *gc.C) {
	err := s.storageBackend.SetFilesystemInfo(s.filesystem.FilesystemTag(), state.FilesystemInfo{
		FilesystemId: "fs-id",
	})
	c.Assert(err, jc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = s.filesystem.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "pending"`)
}
