// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	"github.com/go-goose/goose/v3/nova"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/provider/rackspace"
)

type flavorsSuite struct {
	testing.IsolationSuite
	filter openstack.FlavorFilter
}

var _ = gc.Suite(&flavorsSuite{})

func (s *flavorsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	provider, err := environs.Provider("rackspace")
	c.Assert(err, jc.ErrorIsNil)
	openstackProvider := rackspace.OpenstackProvider(provider)
	s.filter = openstackProvider.FlavorFilter
}

func (s *flavorsSuite) TestFlavorFilter(c *gc.C) {
	s.assertAcceptFlavor(c, "", true)
	s.assertAcceptFlavor(c, "performance1-4", true)
	s.assertAcceptFlavor(c, "compute1-4", false)
	s.assertAcceptFlavor(c, "memory1-15", false)
}

func (s *flavorsSuite) assertAcceptFlavor(c *gc.C, id string, accept bool) {
	accepted := s.filter.AcceptFlavor(nova.FlavorDetail{Id: id})
	c.Assert(accepted, gc.Equals, accept)
}
