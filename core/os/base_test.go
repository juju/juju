// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
)

type baseSuite struct {
	testing.CleanupSuite
}

var _ = tc.Suite(&baseSuite{})

var b = corebase.Base{OS: "freelunch", Channel: corebase.Channel{Track: "0"}}

func (s *baseSuite) TestHostBaseOverride(c *tc.C) {
	// Really just tests that HostBase is overridable
	s.PatchValue(&os.HostBase, func() (corebase.Base, error) {
		return b, nil
	})
	ser, err := os.HostBase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ser, tc.Equals, b)
}
