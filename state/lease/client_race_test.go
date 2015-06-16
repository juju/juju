// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	_ "time"

	_ "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	_ "gopkg.in/mgo.v2/bson"

	_ "github.com/juju/juju/state/lease"
)

// ClientRaceSuite tests the ugliest of details.
type ClientRaceSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientRaceSuite{})

func (s *ClientRaceSuite) TestMany(c *gc.C) {
	c.Fatalf("not done")
}
