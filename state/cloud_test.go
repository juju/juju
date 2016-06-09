// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type CloudConfigSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudConfigSuite{})

func (s *CloudConfigSuite) TestCloudConfig(c *gc.C) {
	settings, err := state.ReadSettings(s.State, state.CloudSettingsC, state.CloudGlobalKey("dummy"))
	c.Assert(err, jc.ErrorIsNil)
	settings.Set("foo", "bar")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := s.State.CloudConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg["foo"], gc.Equals, "bar")
}
