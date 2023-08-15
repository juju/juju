// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/charm/v11"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
)

type channelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&channelSuite{})

func (s channelSuite) TestMakeRiskOnlyChannel(c *gc.C) {
	c.Assert(corecharm.MakeRiskOnlyChannel("edge"), gc.DeepEquals, charm.Channel{
		Track:  "",
		Risk:   "edge",
		Branch: "",
	})
}
