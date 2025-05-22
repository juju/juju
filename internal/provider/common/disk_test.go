// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/provider/common"
)

type DiskSuite struct{}

func TestDiskSuite(t *stdtesting.T) {
	tc.Run(t, &DiskSuite{})
}

func (s *DiskSuite) TestMinRootDiskSizeGiB(c *tc.C) {
	var diskTests = []struct {
		osname       string
		expectedSize uint64
	}{
		{"ubuntu", 8},
		{"centos", 8},
	}
	for _, t := range diskTests {
		actualSize := common.MinRootDiskSizeGiB(ostype.OSTypeForName(t.osname))
		c.Assert(t.expectedSize, tc.Equals, actualSize)
	}
}
