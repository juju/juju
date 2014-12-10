// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type gceSuite struct{}

var _ = gc.Suite(&gceSuite{})

func (*gceSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("gce")
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.Equals, gce.Provider)
}
