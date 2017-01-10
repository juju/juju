// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/cmd/juju/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

func (s *crossmodelSuite) TestAddRelationFromURL(c *gc.C) {
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

func (s *crossmodelSuite) assertAddRelationSameControllerSuccess(c *gc.C, otherModeluser string) {
	_, err := runJujuCommand(c, "add-relation", "wordpress", otherModeluser+"/othermodel.mysql")
	c.Assert(err, jc.ErrorIsNil)
	svc, err := s.State.RemoteApplication("mysql")
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
			ApplicationName: "mysql",
			Relation: charm.Relation{Name: "server",
				Role:      "provider",
				Interface: "mysql",
				Scope:     "global"},
		},
	})
}

func (s *crossmodelSuite) TestAddRelationSameControllerSameOwner(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)

	otherModel := s.Factory.MakeModel(c, &factory.ModelParams{Name: "othermodel"})
	s.AddCleanup(func(*gc.C) { otherModel.Close() })

	mysql := testcharms.Repo.CharmDir("mysql")
	ident := fmt.Sprintf("%s-%d", mysql.Meta().Name, mysql.Revision())
	curl := charm.MustParseURL("local:quantal/" + ident)
	repo, err := charmrepo.InferRepository(
		curl,
		charmrepo.NewCharmStoreParams{},
		testcharms.Repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	ch, err = jujutesting.PutCharm(otherModel, curl, repo, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModel.AddApplication(state.AddApplicationArgs{
		Name:  "mysql",
		Charm: ch,
	})
	s.assertAddRelationSameControllerSuccess(c, "admin")
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionDenied(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)

	otherOwner := s.Factory.MakeUser(c, &factory.UserParams{Name: "otheruser"})
	otherModel := s.Factory.MakeModel(c, &factory.ModelParams{Name: "othermodel", Owner: otherOwner.Tag()})
	s.AddCleanup(func(*gc.C) { otherModel.Close() })

	mysql := testcharms.Repo.CharmDir("mysql")
	ident := fmt.Sprintf("%s-%d", mysql.Meta().Name, mysql.Revision())
	curl := charm.MustParseURL("local:quantal/" + ident)
	repo, err := charmrepo.InferRepository(
		curl,
		charmrepo.NewCharmStoreParams{},
		testcharms.Repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	ch, err = jujutesting.PutCharm(otherModel, curl, repo, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModel.AddApplication(state.AddApplicationArgs{
		Name:  "mysql",
		Charm: ch,
	})

	context, err := runJujuCommand(c, "add-relation", "wordpress", "otheruser/othermodel.mysql")
	c.Assert(err, gc.NotNil)
	c.Assert(testing.Stderr(context), jc.Contains, "You do not have permission to add a relation")
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionAllowed(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)

	otherOwner := s.Factory.MakeUser(c, &factory.UserParams{Name: "otheruser"})
	otherModel := s.Factory.MakeModel(c, &factory.ModelParams{Name: "othermodel", Owner: otherOwner.Tag()})
	s.AddCleanup(func(*gc.C) { otherModel.Close() })

	// Users with write permission to the model can add relations.
	otherFactory := factory.NewFactory(otherModel)
	otherFactory.MakeModelUser(c, &factory.ModelUserParams{User: "admin", Access: permission.WriteAccess})

	mysql := testcharms.Repo.CharmDir("mysql")
	ident := fmt.Sprintf("%s-%d", mysql.Meta().Name, mysql.Revision())
	curl := charm.MustParseURL("local:quantal/" + ident)
	repo, err := charmrepo.InferRepository(
		curl,
		charmrepo.NewCharmStoreParams{},
		testcharms.Repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	ch, err = jujutesting.PutCharm(otherModel, curl, repo, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModel.AddApplication(state.AddApplicationArgs{
		Name:  "mysql",
		Charm: ch,
	})
	s.assertAddRelationSameControllerSuccess(c, "otheruser")
}
