// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
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

func (s *diskSuite) TestDiskSpecNewAttached(c *gc.C) {
	attached := google.NewAttached(s.DiskSpec)

	c.Check(attached.Type, gc.Equals, "PERSISTENT")
	c.Check(attached.Boot, jc.IsTrue)
	c.Check(attached.Mode, gc.Equals, "READ_WRITE")
	c.Check(attached.AutoDelete, jc.IsTrue)
	c.Check(attached.Interface, gc.Equals, "")
	c.Check(attached.DeviceName, gc.Equals, "")
}

func (s *diskSuite) TestRootDisk(c *gc.C) {
	attached := google.RootDisk(&s.Instance)
	c.Assert(attached, gc.NotNil)

	c.Check(attached.Type, gc.Equals, "PERSISTENT")
	c.Check(attached.Boot, jc.IsTrue)
	c.Check(attached.Mode, gc.Equals, "READ_WRITE")
	c.Check(attached.AutoDelete, jc.IsTrue)
	c.Check(attached.Interface, gc.Equals, "")
	c.Check(attached.DeviceName, gc.Equals, "")
}

func (s *diskSuite) TestDiskSizeGB(c *gc.C) {
	attached := google.NewAttached(s.DiskSpec)
	size, err := google.DiskSizeGB(attached)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(size, gc.Equals, int64(1))
}
