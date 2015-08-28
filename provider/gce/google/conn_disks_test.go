// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	//"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

const fakeVolName = "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"

func fakeDiskAndSpec() (google.DiskSpec, *compute.Disk, error) {
	spec := google.DiskSpec{
		SizeHintGB:         1,
		Name:               fakeVolName,
		PersistentDiskType: google.DiskPersistentSSD,
	}
	fakeDisk, err := google.NewDetached(spec)
	return spec, fakeDisk, err
}

func (s *connSuite) TestConnectionCreateDisks(c *gc.C) {
	spec, _, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)

	disks, err := s.Conn.CreateDisks("home-zone", []google.DiskSpec{spec})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, gc.HasLen, 1)
	c.Assert(disks[0].Name, gc.Equals, fakeVolName)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "CreateDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].ComputeDisk.Name, gc.Equals, fakeVolName)
}

func (s *connSuite) TestConnectionDisks(c *gc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disks = []*compute.Disk{fakeDisk}

	disks, err := s.Conn.Disks("home-zone")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, gc.HasLen, 1)
	fakeGoogleDisk := google.NewDisk(fakeDisk)
	c.Assert(disks[0], gc.DeepEquals, fakeGoogleDisk)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "ListDisks")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
}

func (s *connSuite) TestConnectionDisk(c *gc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	disk, err := s.Conn.Disk("home-zone", fakeVolName)
	c.Check(err, jc.ErrorIsNil)
	fakeGoogleDisk := google.NewDisk(fakeDisk)
	c.Assert(disk, gc.DeepEquals, fakeGoogleDisk)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
}

func (s *connSuite) TestConnectionAttachDisk(c *gc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	att, err := s.Conn.AttachDisk("home-zone", fakeVolName, "a-fake-instance", google.ModeRW)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")

	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "AttachDisk")
	c.Check(s.FakeConn.Calls[1].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, gc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[1].InstanceId, gc.Equals, "a-fake-instance")
	c.Check(s.FakeConn.Calls[1].AttachedDisk.DeviceName, gc.Equals, att.DeviceName)
	c.Check(s.FakeConn.Calls[1].AttachedDisk.Mode, gc.Equals, string(att.Mode))

}

func (s *connSuite) TestConnectionDetachDisk(c *gc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	err = s.Conn.DetachDisk("home-zone", "a-fake-instance", fakeVolName)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")

	c.Check(s.FakeConn.Calls[1].FuncName, gc.Equals, "DetachDisk")
	c.Check(s.FakeConn.Calls[1].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, gc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[1].InstanceId, gc.Equals, "a-fake-instance")
	c.Check(s.FakeConn.Calls[1].DeviceName, gc.Equals, "home-zone-0")
}

func (s *connSuite) TestConnectionRemoveDisks(c *gc.C) {
	err := s.Conn.RemoveDisk("home-zone", fakeVolName)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "RemoveDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].ID, gc.Equals, fakeVolName)
}

func (s *connSuite) TestConnectionInstanceDisks(c *gc.C) {
	s.FakeConn.AttachedDisks = []*compute.AttachedDisk{{
		Source:     "https://bogus/url/project/aproject/zone/azone/disk/" + fakeVolName,
		DeviceName: "home-zone-0",
		Mode:       string(google.ModeRW),
	}}
	disks, err := s.Conn.InstanceDisks("home-zone", "a-fake-instance")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, gc.HasLen, 1)
	c.Assert(disks[0].VolumeName, gc.Equals, fakeVolName)
	c.Assert(disks[0].DeviceName, gc.Equals, "home-zone-0")
	c.Assert(disks[0].Mode, gc.Equals, google.ModeRW)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "InstanceDisks")
	c.Check(s.FakeConn.Calls[0].ProjectID, gc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, gc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].InstanceId, gc.Equals, "a-fake-instance")
}
