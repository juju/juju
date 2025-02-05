// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostname_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/hostname"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type HostnameSuite struct{}

var _ = gc.Suite(&HostnameSuite{})

// TestParseHostname tests that the getter method return the expected values.
func (s *HostnameSuite) TestParseHostname(c *gc.C) {
	res, err := hostname.ParseHostname("charm.1.postgresql.8419cd78-4993-4c3a-928e-c646226beeee.juju.local")
	c.Assert(err, gc.IsNil)
	c.Check(res.Unit(), gc.Equals, "postgresql/1")
	c.Check(res.Application(), gc.Equals, "postgresql")
	c.Check(res.Container(), gc.Equals, "charm")
	c.Check(res.ModelUUID(), gc.Equals, "8419cd78-4993-4c3a-928e-c646226beeee")
	c.Check(res.Machine(), gc.Equals, 0)
	c.Check(res.Target(), gc.Equals, hostname.ContainerTarget)
}
