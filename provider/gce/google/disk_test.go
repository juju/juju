// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	jc "github.com/juju/testing/checkers"
	"google.golang.org/api/compute/v1"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type diskSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&diskSuite{})

func (s *diskSuite) TestDiskSpecTooSmall(c *gc.C) {
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, jc.IsFalse)
}

func (s *diskSuite) TestDiskSpecTooSmallTrue(c *gc.C) {
	s.DiskSpec.SizeHintGB = 0
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, jc.IsTrue)
}

func (s *diskSuite) TestDiskSpecSizeGB(c *gc.C) {
	size := s.DiskSpec.SizeGB()

	c.Check(size, gc.Equals, uint64(15))
}

func (s *diskSuite) TestDiskSpecSizeGBMin(c *gc.C) {
	s.DiskSpec.SizeHintGB = 0
	size := s.DiskSpec.SizeGB()

	c.Check(size, gc.Equals, uint64(10))
}

type attachedInfo struct {
	attached *compute.AttachedDisk
	diskType string
	diskMode string
}

func (s *diskSuite) checkAttached(c *gc.C, aInfo attachedInfo) {
	c.Check(aInfo.attached.Type, gc.Equals, aInfo.diskType)
	c.Check(aInfo.attached.Boot, gc.Equals, s.DiskSpec.Boot)
	c.Check(aInfo.attached.Mode, gc.Equals, aInfo.diskMode)
	c.Check(aInfo.attached.AutoDelete, gc.Equals, s.DiskSpec.AutoDelete)
	c.Check(aInfo.attached.Interface, gc.Equals, "")
	c.Check(aInfo.attached.DeviceName, gc.Equals, "")
}

func (s *diskSuite) TestDiskSpecNewAttached(c *gc.C) {
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedBootFalse(c *gc.C) {
	s.DiskSpec.Boot = false
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedAutoDeleteFalse(c *gc.C) {
	s.DiskSpec.AutoDelete = false
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedScratch(c *gc.C) {
	s.DiskSpec.Scratch = true
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "SCRATCH",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestDiskSpecNewAttachedReadOnly(c *gc.C) {
	s.DiskSpec.Readonly = true
	attached := google.NewAttached(s.DiskSpec)

	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_ONLY",
	})
}

func (s *diskSuite) TestRootDiskInstance(c *gc.C) {
	attached := s.Instance.RootDisk()

	c.Assert(attached, gc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestRootDiskInstanceNilSpec(c *gc.C) {
	inst := google.Instance{}
	attached := inst.RootDisk()

	c.Assert(attached, gc.IsNil)
}

func (s *diskSuite) TestRootDiskSpec(c *gc.C) {
	attached := s.InstanceSpec.RootDisk()

	c.Assert(attached, gc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}
