// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type diskSuite struct {
	google.BaseSuite
	spec google.DiskSpec
}

var _ = gc.Suite(&diskSuite{})

func (s *diskSuite) TestDiskSpecTooSmall(c *gc.C) {
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, jc.IsFalse)
}

func (s *diskSuite) TestDiskSpecTooSmallTrue(c *gc.C) {
	s.DiskSpec.SizeHintGB = -1
	tooSmall := s.DiskSpec.TooSmall()

	c.Check(tooSmall, jc.IsTrue)
}

func (s *diskSuite) TestDiskSpecSizeGB(c *gc.C) {
	size := s.DiskSpec.SizeGB()

	c.Check(size, gc.Equals, int64(1))
}

func (s *diskSuite) TestDiskSpecSizeGBMin(c *gc.C) {
	s.DiskSpec.SizeHintGB = -1
	size := s.DiskSpec.SizeGB()

	c.Check(size, gc.Equals, int64(0))
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

func (s *diskSuite) TestRootDiskCompute(c *gc.C) {
	attached := google.RootDisk(&s.RawInstance)

	c.Assert(attached, gc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestRootDiskComputeValue(c *gc.C) {
	attached := google.RootDisk(s.RawInstance)

	c.Assert(attached, gc.IsNil)
}

func (s *diskSuite) TestRootDiskInstance(c *gc.C) {
	attached := google.RootDisk(&s.Instance)

	c.Assert(attached, gc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestRootDiskInstanceNilSpec(c *gc.C) {
	attached := google.RootDisk(&google.Instance{})

	c.Assert(attached, gc.IsNil)
}

func (s *diskSuite) TestRootDiskSpec(c *gc.C) {
	attached := google.RootDisk(&s.InstanceSpec)

	c.Assert(attached, gc.NotNil)
	s.checkAttached(c, attachedInfo{
		attached: attached,
		diskType: "PERSISTENT",
		diskMode: "READ_WRITE",
	})
}

func (s *diskSuite) TestRootDiskUnknown(c *gc.C) {
	attached := google.RootDisk("hello")

	c.Assert(attached, gc.IsNil)
}

func (s *diskSuite) TestDiskSizeGB(c *gc.C) {
	attached := google.NewAttached(s.DiskSpec)
	size, err := google.DiskSizeGB(attached)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(size, gc.Equals, int64(1))
}
