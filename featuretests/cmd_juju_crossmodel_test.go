// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/cmd/juju/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type crossmodelSuite struct {
	jujutesting.JujuConnSuite
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
    store: local
    url: /u/me/riak
    endpoints:
      endpoint:
        interface: http
        role: provider
  varnish:
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
