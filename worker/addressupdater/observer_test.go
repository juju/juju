// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addressupdater

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/testing"
)

var _ = gc.Suite(&observerSuite{})

type observerSuite struct {
	testing.JujuConnSuite
}

func (s *observerSuite) TestWaitsForValidEnviron(c *gc.C) {
	obs, err := newEnvironObserver(s.State, nil)
	c.Assert(err, gc.IsNil)
	env := obs.Environ()
	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(env.Config().AllAttrs(), gc.DeepEquals, stateConfig.AllAttrs())
}
