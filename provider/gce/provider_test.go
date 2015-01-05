// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
)

type providerSuite struct{}

var _ = gc.Suite(&providerSuite{})

func (*providerSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("gce")
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.Equals, gce.Provider)
}
