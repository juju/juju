// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/juju/provider/common"
	gc "gopkg.in/check.v1"
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
		{"fake-series", 8},
	}
	for _, t := range diskTests {
		actualSize := common.MinRootDiskSizeGiB(t.series)
		c.Assert(t.expectedSize, gc.Equals, actualSize)
	}
}
