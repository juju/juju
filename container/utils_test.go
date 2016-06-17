// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&UtilsSuite{})

type UtilsSuite struct {
	testing.BaseSuite
}

func (s *UtilsSuite) TestIsLXCSupportedOnHost(c *gc.C) {
	s.PatchValue(&RunningInContainer, func() bool {
		return false
	})
	supports := ContainersSupported()
	c.Assert(supports, jc.IsTrue)
}

func (s *UtilsSuite) TestIsLXCSupportedOnLXCContainer(c *gc.C) {
	s.PatchValue(&RunningInContainer, func() bool {
		return true
	})
	supports := ContainersSupported()
	c.Assert(supports, jc.IsFalse)
}
