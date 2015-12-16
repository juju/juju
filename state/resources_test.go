// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/component/all"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

var _ = gc.Suite(&ResourcesSuite{})

type ResourcesSuite struct {
	ConnSuite
}

func (s *ResourcesSuite) TestFunctional(c *gc.C) {
	st, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)

	resources, err := st.ListResources("a-service")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(resources, gc.HasLen, 0)

	// TODO(ericsnow) Add more as state.Resources grows more functionality.
}
