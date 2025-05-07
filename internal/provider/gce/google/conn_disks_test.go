// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

const fakeVolName = "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4"

func fakeDiskAndSpec() (google.DiskSpec, *compute.Disk, error) {
	spec := google.DiskSpec{
		OS:                 "ubuntu",
		SizeHintGB:         1,
		Name:               fakeVolName,
		PersistentDiskType: google.DiskPersistentSSD,
	}
	fakeDisk, err := google.NewDetached(spec)
	return spec, fakeDisk, err
}

func (s *connSuite) TestConnectionCreateDisks(c *tc.C) {
	spec, _, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)

	disks, err := s.Conn.CreateDisks("home-zone", []google.DiskSpec{spec})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, tc.HasLen, 1)
	c.Assert(disks[0].Name, tc.Equals, fakeVolName)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "CreateDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].ComputeDisk.Name, tc.Equals, fakeVolName)
}

func (s *connSuite) TestConnectionDisks(c *tc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disks = []*compute.Disk{fakeDisk}

	disks, err := s.Conn.Disks()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, tc.HasLen, 1)
	fakeGoogleDisk := google.NewDisk(fakeDisk)
	c.Assert(disks[0], tc.DeepEquals, fakeGoogleDisk)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "ListDisks")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "")
}

func (s *connSuite) TestConnectionDisk(c *tc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	s.FakeConn.Disk.Zone = "https://www.googleapis.com/compute/v1/projects/my-project/zones/home-zone"

	disk, err := s.Conn.Disk("home-zone", fakeVolName)
	c.Check(err, jc.ErrorIsNil)
	fakeGoogleDisk := google.NewDisk(fakeDisk)
	c.Assert(disk, tc.DeepEquals, fakeGoogleDisk)
	c.Assert(disk.Zone, tc.Equals, "home-zone")

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")
}

func (s *connSuite) TestConnectionSetDiskLabels(c *tc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	labels := map[string]string{
		"a": "b",
		"c": "d",
	}
	err = s.Conn.SetDiskLabels("home-zone", fakeVolName, "fingerprint", labels)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "SetDiskLabels")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].LabelFingerprint, tc.Equals, "fingerprint")
	c.Check(s.FakeConn.Calls[0].Labels, jc.DeepEquals, labels)
}

func (s *connSuite) TestConnectionAttachDisk(c *tc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	att, err := s.Conn.AttachDisk("home-zone", fakeVolName, "a-fake-instance", google.ModeRW)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")

	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "AttachDisk")
	c.Check(s.FakeConn.Calls[1].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[1].InstanceId, tc.Equals, "a-fake-instance")
	c.Check(s.FakeConn.Calls[1].AttachedDisk.DeviceName, tc.Equals, att.DeviceName)
	c.Check(s.FakeConn.Calls[1].AttachedDisk.Mode, tc.Equals, string(att.Mode))

}

func (s *connSuite) TestConnectionDetachDisk(c *tc.C) {
	_, fakeDisk, err := fakeDiskAndSpec()
	c.Check(err, jc.ErrorIsNil)
	s.FakeConn.Disk = fakeDisk
	err = s.Conn.DetachDisk("home-zone", "a-fake-instance", fakeVolName)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 2)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "GetDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")

	c.Check(s.FakeConn.Calls[1].FuncName, tc.Equals, "DetachDisk")
	c.Check(s.FakeConn.Calls[1].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[1].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[1].InstanceId, tc.Equals, "a-fake-instance")
	c.Check(s.FakeConn.Calls[1].DeviceName, tc.Equals, "home-zone-0")
}

func (s *connSuite) TestConnectionRemoveDisks(c *tc.C) {
	err := s.Conn.RemoveDisk("home-zone", fakeVolName)
	c.Check(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "RemoveDisk")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].ID, tc.Equals, fakeVolName)
}

func (s *connSuite) TestConnectionInstanceDisks(c *tc.C) {
	s.FakeConn.AttachedDisks = []*compute.AttachedDisk{{
		Source:     "https://bogus/url/project/aproject/zone/azone/disk/" + fakeVolName,
		DeviceName: "home-zone-0",
		Mode:       string(google.ModeRW),
	}}
	disks, err := s.Conn.InstanceDisks("home-zone", "a-fake-instance")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(disks, tc.HasLen, 1)
	c.Assert(disks[0].VolumeName, tc.Equals, fakeVolName)
	c.Assert(disks[0].DeviceName, tc.Equals, "home-zone-0")
	c.Assert(disks[0].Mode, tc.Equals, google.ModeRW)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "InstanceDisks")
	c.Check(s.FakeConn.Calls[0].ProjectID, tc.Equals, "spam")
	c.Check(s.FakeConn.Calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(s.FakeConn.Calls[0].InstanceId, tc.Equals, "a-fake-instance")
}
