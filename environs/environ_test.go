// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
)

type environSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestGetEnvironment(c *gc.C) {
	env, err := environs.GetEnviron(s.State, environs.New)
	c.Assert(err, jc.ErrorIsNil)
	config, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env.Config().UUID(), jc.DeepEquals, config.UUID())
	c.Check(env.Config().ControllerUUID(), jc.DeepEquals, config.ControllerUUID())
	c.Check(env, gc.Not(gc.Equals), s.Environ)
}
