// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/internal/provider/gce/google"
)

type diskSuite struct {
	google.BaseSuite
}

func TestDiskSuite(t *stdtesting.T) {
	tc.Run(t, &diskSuite{})
}

func (s *diskSuite) TestDiskSpecTooSmall(c *tc.C) {
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, tc.IsFalse)
}

func (s *diskSuite) TestDiskSpecTooSmallTrue(c *tc.C) {
	s.DiskSpec.SizeHintGB = 0
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, tc.IsTrue)
}

func (s *diskSuite) TestDiskSpecSizeGB(c *tc.C) {
	size := s.DiskSpec.SizeGB()

	c.Check(size, tc.Equals, uint64(15))
}

func (s *diskSuite) TestDiskSpecSizeGBMinUbuntu(c *tc.C) {
	s.DiskSpec.SizeHintGB = 0
	size := s.DiskSpec.SizeGB()

	c.Check(size, tc.Equals, uint64(10))
}

func (s *diskSuite) TestDiskSpecSizeGBMinUnknown(c *tc.C) {
	s.DiskSpec.SizeHintGB = 0
	s.DiskSpec.OS = "centos"
	size := s.DiskSpec.SizeGB()

	c.Check(size, tc.Equals, uint64(10))
}

type attachedInfo struct {
	attached *compute.AttachedDisk
	diskType string
	diskMode string
}

func (s *diskSuite) checkAttached(c *tc.C, aInfo attachedInfo) {
	c.Check(aInfo.attached.Type, tc.Equals, aInfo.diskType)
	c.Check(aInfo.attached.Boot, tc.Equals, s.DiskSpec.Boot)
	c.Check(aInfo.attached.Mode, tc.Equals, aInfo.diskMode)
	c.Check(aInfo.attached.AutoDelete, tc.Equals, s.DiskSpec.AutoDelete)
	c.Check(aInfo.attached.Interface, tc.Equals, "")
	c.Check(aInfo.attached.DeviceName, tc.Equals, "")
}

func (s *diskSuite) TestDiskSpecNewAttached(c *tc.C) {
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedBootFalse(c *tc.C) {
	s.DiskSpec.Boot = false
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedAutoDeleteFalse(c *tc.C) {
	s.DiskSpec.AutoDelete = false
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedScratch(c *tc.C) {
	s.DiskSpec.Scratch = true
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "SCRATCH",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedReadOnly(c *tc.C) {
	s.DiskSpec.Readonly = true
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_ONLY",
	})
}

func (s *diskSuite) TestRootDiskInstance(c *tc.C) {
	attached := s.Instance.RootDisk()

	c.Assert(attached, tc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestRootDiskInstanceNilSpec(c *tc.C) {
	inst := google.Instance{}
	attached := inst.RootDisk()

	c.Assert(attached, tc.IsNil)
}

func (s *diskSuite) TestRootDiskSpec(c *tc.C) {
	attached := s.InstanceSpec.RootDisk()

	c.Assert(attached, tc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}
