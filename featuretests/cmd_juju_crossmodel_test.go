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
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/model"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
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

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(), "riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(), "varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := testing.RunCommand(c, crossmodel.NewListEndpointsCommand(),
		"--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
riak:
  store: kontroll
  charm: riak
  url: admin/controller.riak
  endpoints:
    endpoint:
      interface: http
      role: provider
varnish:
  store: kontroll
  charm: varnish
  url: admin/controller.varnish
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
		"riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := testing.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"admin/controller.varnish", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
admin/controller.varnish:
  endpoints:
    webcache:
      interface: varnish
      role: provider
  description: Another popular database
`[1:])
}

func (s *crossmodelSuite) TestShowOtherModel(c *gc.C) {
	s.addOtherModelApplication(c)

	ctx, err := testing.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"otheruser/othermodel.hosted-mysql", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
otheruser/othermodel.hosted-mysql:
  endpoints:
    database:
      interface: mysql
      role: provider
`[1:])
}

func (s *crossmodelSuite) setupOffers(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingService(c, "varnishservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)
}
func (s *crossmodelSuite) TestFind(c *gc.C) {
	s.setupOffers(c)
	ctx, err := testing.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"admin/controller", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
admin/controller.riak:
  endpoints:
    endpoint:
      interface: http
      role: provider
admin/controller.varnish:
  endpoints:
    webcache:
      interface: varnish
      role: provider
`[1:])
}

func (s *crossmodelSuite) TestFindOtherModel(c *gc.C) {
	s.addOtherModelApplication(c)

	ctx, err := testing.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"otheruser/othermodel", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
otheruser/othermodel.hosted-mysql:
  endpoints:
    database:
      interface: mysql
      role: provider
`[1:])
}

func (s *crossmodelSuite) TestFindAllModels(c *gc.C) {
	s.setupOffers(c)
	s.addOtherModelApplication(c)

	ctx, err := testing.RunCommand(c, crossmodel.NewFindEndpointsCommand(), "kontroll:", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
admin/controller.riak:
  endpoints:
    endpoint:
      interface: http
      role: provider
admin/controller.varnish:
  endpoints:
    webcache:
      interface: varnish
      role: provider
otheruser/othermodel.hosted-mysql:
  endpoints:
    database:
      interface: mysql
      role: provider
`[1:])
}

func (s *crossmodelSuite) TestAddRelationFromURL(c *gc.C) {
	c.Skip("add relation from URL not currently supported")

	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)
	ch = s.AddTestingCharm(c, "mysql")
	s.AddTestingService(c, "mysql", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(),
		"mysql:server", "me/model.hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runJujuCommand(c, "add-relation", "wordpress", "me/model.hosted-mysql")
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
	_, err := runJujuCommand(c, "add-relation", "wordpress", otherModeluser+"/othermodel.hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := app.Relations()
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
	c.Assert(err, jc.ErrorIsNil)
	offersAPi := state.NewApplicationOffers(otherModel)
	_, err = offersAPi.AddOffer(jujucrossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"database": "server"},
		Owner:           s.AdminUserTag(c).Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddRelationSameControllerSuccess(c, "admin")
}

func (s *crossmodelSuite) addOtherModelApplication(c *gc.C) *state.State {
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
	ch, err := jujutesting.PutCharm(otherModel, curl, repo, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = otherModel.AddApplication(state.AddApplicationArgs{
		Name:  "mysql",
		Charm: ch,
	})

	offersAPi := state.NewApplicationOffers(otherModel)
	_, err = offersAPi.AddOffer(jujucrossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"database": "server"},
		Owner:           otherOwner.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return otherModel
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionDenied(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)
	s.addOtherModelApplication(c)

	context, err := runJujuCommand(c, "add-relation", "wordpress", "otheruser/othermodel.mysql")
	c.Assert(err, gc.NotNil)
	c.Assert(testing.Stderr(context), jc.Contains, "You do not have permission to add a relation")
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionAllowed(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, "wordpress", ch)

	otherModel := s.addOtherModelApplication(c)
	// Users with write permission to the model can add relations.
	otherFactory := factory.NewFactory(otherModel)
	otherFactory.MakeModelUser(c, &factory.ModelUserParams{User: "admin", Access: permission.WriteAccess})

	s.assertAddRelationSameControllerSuccess(c, "otheruser")
}

func (s *crossmodelSuite) assertOfferGrant(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingService(c, "riakservice", ch)

	_, err := testing.RunCommand(c, crossmodel.NewOfferCommand(), "riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)

	// Check the default access levels.
	offerTag := names.NewApplicationOfferTag("riak")
	userTag := names.NewUserTag("everyone@external")
	access, err := s.State.GetOfferAccess(offerTag, userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
	access, err = s.State.GetOfferAccess(offerTag, names.NewUserTag("admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Grant consume access.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "bob"})
	_, err = testing.RunCommand(c, model.NewGrantCommand(), "bob", "consume", "admin/controller.riak")
	c.Assert(err, jc.ErrorIsNil)
	access, err = s.State.GetOfferAccess(offerTag, names.NewUserTag("bob"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ConsumeAccess)

}

func (s *crossmodelSuite) TestOfferGrant(c *gc.C) {
	s.assertOfferGrant(c)
}

func (s *crossmodelSuite) TestOfferRevoke(c *gc.C) {
	s.assertOfferGrant(c)
	offerTag := names.NewApplicationOfferTag("riak")

	// Revoke consume access.
	_, err := testing.RunCommand(c, model.NewRevokeCommand(), "bob", "consume", "admin/controller.riak")
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.GetOfferAccess(offerTag, names.NewUserTag("bob"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}
