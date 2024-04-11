// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package base_test

import (
	"github.com/juju/juju/core/os/base"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
)

type baseSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&baseSuite{})

var b = corebase.Base{OS: "freelunch", Channel: corebase.Channel{Track: "0"}}

func (s *baseSuite) TestHostBaseOverride(c *gc.C) {
	// Really just tests that HostBase is overridable
	s.PatchValue(&base.HostBase, func() (corebase.Base, error) {
		return b, nil
	})
	ser, err := base.HostBase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ser, gc.Equals, b)
}
