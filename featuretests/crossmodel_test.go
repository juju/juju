// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	apicrossmodel "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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
	offersApi := state.NewOfferedApplications(s.State)
	offers, err := offersApi.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0], jc.DeepEquals, jujucrossmodel.OfferedApplication{
		ApplicationName: "riakservice",
		ApplicationURL:  "local:/u/admin/controller/riakservice",
		CharmName:       "riak",
		Endpoints:       map[string]string{"endpoint": "endpoint"},
		Description:     "Scalable K/V Store in Erlang with Clocks :-)",
		Registered:      true,
	})
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint,admin", "local:/u/me/service")
	c.Assert(err, jc.ErrorIsNil)
	offersApi := state.NewOfferedApplications(s.State)
	offers, err := offersApi.ListOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 1)
	c.Assert(offers[0], jc.DeepEquals, jujucrossmodel.OfferedApplication{
		ApplicationName: "riakservice",
		ApplicationURL:  "local:/u/me/service",
		CharmName:       "riak",
		Endpoints:       map[string]string{"admin": "admin", "endpoint": "endpoint"},
		Description:     "Scalable K/V Store in Erlang with Clocks :-)",
		Registered:      true,
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

func (s *crossmodelSuite) TestLocalURLOtherEnvironment(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)
	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "local:/u/me/riak")
	c.Assert(err, jc.ErrorIsNil)

	user := s.Factory.MakeUser(c, &factory.UserParams{
		NoModelUser: true,
		Password:    "super-secret",
	})
	otherState := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "first", Owner: user.Tag()})
	defer otherState.Close()

	info := s.APIInfo(c)
	info.ModelTag = otherState.ModelTag()
	info.Tag = user.Tag()
	info.Password = "super-secret"
	otherAPIState, err := api.Open(info, api.DefaultDialOpts())
	c.Assert(err, jc.ErrorIsNil)
	defer otherAPIState.Close()

	apiClient := apicrossmodel.NewClient(otherAPIState)
	offer, err := apiClient.ApplicationOffer("local:/u/me/riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer, jc.DeepEquals, params.ApplicationOffer{
		ApplicationURL:         "local:/u/me/riak",
		SourceModelTag:         "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		SourceLabel:            "controller",
		ApplicationName:        "riakservice",
		ApplicationDescription: "Scalable K/V Store in Erlang with Clocks :-)",
		Endpoints: []params.RemoteEndpoint{
			{Name: "endpoint", Role: "provider", Interface: "http", Scope: "global"},
		},
	})
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

func (s *crossmodelSuite) TestFind(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingService(c, "varnishservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "local:/u/you/riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "local:/u/me/varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - find with interface and endpoint name filters when supported
	ctx, err := testing.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"local:/u/me", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
local:/u/me/varnish:
  endpoints:
    webcache:
      interface: varnish
      role: provider
`[1:])
}

func (s *crossmodelSuite) TestAddRelation(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)
	ch = s.AddTestingCharm(c, "mysql")
	s.AddTestingService(c, "mysql", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"mysql:server", "local:/u/me/hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runJujuCommand(c, "add-relation", "wordpress", "local:/u/me/hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	svc, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel, gc.HasLen, 1)
	c.Assert(rel[0].Endpoints(), jc.SameContents, []state.Endpoint{
		{
			ApplicationName: "wordpress",
			Relation: charm.Relation{
				Name:      "db",
				Role:      "requirer",
				Interface: "mysql",
				Limit:     1,
				Scope:     "global",
			},
		}, {
			ApplicationName: "hosted-mysql",
			Relation: charm.Relation{Name: "server",
				Role:      "provider",
				Interface: "mysql",
				Scope:     "global"},
		},
	})
}
