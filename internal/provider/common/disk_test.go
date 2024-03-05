// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/os"
	"github.com/juju/juju/internal/provider/common"
)

type DiskSuite struct{}

var _ = gc.Suite(&DiskSuite{})

func (s *DiskSuite) TestMinRootDiskSizeGiB(c *gc.C) {
	var diskTests = []struct {
		osname       string
		expectedSize uint64
	}{
		{"ubuntu", 8},
		{"centos", 8},
	}
	for _, t := range diskTests {
		actualSize := common.MinRootDiskSizeGiB(os.OSTypeForName(t.osname))
		c.Assert(t.expectedSize, gc.Equals, actualSize)
	}
}
