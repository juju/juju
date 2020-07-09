// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/common"
)

type DiskSuite struct{}

var _ = gc.Suite(&DiskSuite{})

func (s *DiskSuite) TestMinRootDiskSizeGiB(c *gc.C) {
	var diskTests = []struct {
		series       string
		expectedSize uint64
	}{
		{"trusty", 8},
		{"win2012r2", 40},
		{"centos7", 8},
		{"centos8", 8},
		{"opensuseleap", 8},
		{"fake-series", 8},
	}
	for _, t := range diskTests {
		actualSize := common.MinRootDiskSizeGiB(t.series)
		c.Assert(t.expectedSize, gc.Equals, actualSize)
	}
}
