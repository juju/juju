// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type crossmodelSuite struct {
	jujutesting.JujuConnSuite
}

func (s *crossmodelSuite) TestOfferDefaultURL(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint")
	c.Assert(err, jc.ErrorIsNil)
	offersApi := state.NewOfferedServices(s.State)
	offers, err := offersApi.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0], jc.DeepEquals, jujucrossmodel.OfferedService{
		ServiceName: "riakservice",
		ServiceURL:  "local:/u/dummy-admin/dummyenv/riakservice",
		CharmName:   "riak",
		Endpoints:   map[string]string{"endpoint": "endpoint"},
		Description: "Scalable K/V Store in Erlang with Clocks :-)",
		Registered:  true,
	})
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint,admin", "local:/u/me/service")
	c.Assert(err, jc.ErrorIsNil)
	offersApi := state.NewOfferedServices(s.State)
	offers, err := offersApi.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0], jc.DeepEquals, jujucrossmodel.OfferedService{
		ServiceName: "riakservice",
		ServiceURL:  "local:/u/me/service",
		CharmName:   "riak",
		Endpoints:   map[string]string{"admin": "admin", "endpoint": "endpoint"},
		Description: "Scalable K/V Store in Erlang with Clocks :-)",
		Registered:  true,
	})
}

func (s *crossmodelSuite) TestListEndpoints(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingService(c, "varnishservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "local:/u/me/riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "local:/u/me/varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := testing.RunCommand(c, crossmodel.NewListEndpointsCommand(),
		"--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
local:
  riak:
    charm: riak
    connected: 0
    store: local
    url: /u/me/riak
    endpoints:
      endpoint:
        interface: http
        role: provider
  varnish:
    charm: varnish
    connected: 0
    store: local
    url: /u/me/varnish
    endpoints:
      webcache:
        interface: varnish
        role: provider
`[1:])
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingService(c, "varnishservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "local:/u/me/riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "local:/u/me/varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := testing.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"local:/u/me/varnish", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
varnishservice:
  endpoints:
    webcache:
      interface: varnish
      role: provider
  description: Another popular database
`[1:])
}
