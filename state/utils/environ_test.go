// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/utils"
)

type environSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestGetEnvironment(c *gc.C) {
	env, err := utils.GetEnvironment(s.State)
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env.Config(), jc.DeepEquals, config)
	c.Check(env, gc.Not(gc.Equals), s.Environ)
}
