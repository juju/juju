// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type lxdStateCharmProfilerSuite struct{}

var _ = gc.Suite(&lxdStateCharmProfilerSuite{})

func (*lxdStateCharmProfilerSuite) TestLXDProfileEmptyCharm(c *gc.C) {
	wrapper := lxdStateCharmProfiler{
		Charm: nil,
	}
	c.Check(wrapper.LXDProfile(), gc.IsNil)
}

func (*lxdStateCharmProfilerSuite) TestLXDProfileCharmNoProfile(c *gc.C) {
	wrapper := lxdStateCharmProfiler{
		Charm: &state.Charm{},
	}
	c.Check(wrapper.LXDProfile(), gc.IsNil)
}
