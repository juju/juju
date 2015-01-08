// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google_test

import (
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/gce/google"
)

type diskSuite struct {
	google.BaseSuite
}

var _ = gc.Suite(&diskSuite{})

func (s *diskSuite) TestDiskSpecTooSmall(c *gc.C) {
	//spec := google.DiskSpec{
	//	SizeHintGB: 1,
	//}
	//tooSmall := spec.TooSmall()

	//c.Check(tooSmall, jc.IsFalse)
}

func (s *diskSuite) TestDiskSpecSizeGB(c *gc.C) {
}

func (s *diskSuite) TestDiskSpecNewAttached(c *gc.C) {
}

func (s *diskSuite) TestRootDisk(c *gc.C) {
}

func (s *diskSuite) TestDiskSizeGB(c *gc.C) {
}
