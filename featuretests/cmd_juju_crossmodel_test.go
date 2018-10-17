// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/cmd/juju/model"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing/factory"
)

type crossmodelSuite struct {
	jujutesting.JujuConnSuite
}

func (s *crossmodelSuite) TestListEndpoints(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingApplication(c, "varnishservice", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(), "riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(), "varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewListEndpointsCommand(),
		"--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
riak:
  application: riakservice
  store: kontroll
  charm: local:quantal/riak-7
  offer-url: admin/controller.riak
  endpoints:
    endpoint:
      interface: http
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
varnish:
  application: varnishservice
  store: kontroll
  charm: local:quantal/varnish-1
  offer-url: admin/controller.varnish
  endpoints:
    webcache:
      interface: varnish
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
`[1:])
}

func (s *crossmodelSuite) TestListEndpointsOtherModel(c *gc.C) {
	s.addOtherModelApplication(c)

	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewListEndpointsCommand(),
		"-m", "otheruser/othermodel", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
hosted-mysql:
  application: mysql
  store: kontroll
  charm: local:quantal/mysql-1
  offer-url: otheruser/othermodel.hosted-mysql
  endpoints:
    database:
      interface: mysql
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    otheruser:
      display-name: Other
      access: admin
`[1:])
}

func (s *crossmodelSuite) TestRemove(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingApplication(c, "varnishservice", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, crossmodel.NewRemoveOfferCommand(),
		"admin/controller.riak")
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"admin/controller.riak")
	c.Assert(err, gc.ErrorMatches, `application offer "admin/controller\.riak" not found`)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingApplication(c, "varnishservice", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(wallyworld) - list with filters when supported
	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"admin/controller.varnish", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
kontroll:admin/controller.varnish:
  description: Another popular database
  access: admin
  endpoints:
    webcache:
      interface: varnish
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
`[1:])
}

func (s *crossmodelSuite) TestShowOtherModel(c *gc.C) {
	s.addOtherModelApplication(c)

	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewShowOfferedEndpointCommand(),
		"otheruser/othermodel.hosted-mysql", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	otherUser, err := s.State.User(names.NewUserTag("otheruser"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, fmt.Sprintf(`
kontroll:otheruser/othermodel.hosted-mysql:
  access: admin
  endpoints:
    database:
      interface: mysql
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    otheruser:
      display-name: %s
      access: admin
`, otherUser.DisplayName())[1:])
}

func (s *crossmodelSuite) setupOffers(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "riakservice", ch)
	ch = s.AddTestingCharm(c, "varnish")
	s.AddTestingApplication(c, "varnishservice", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"varnishservice:webcache", "varnish")
	c.Assert(err, jc.ErrorIsNil)
}
func (s *crossmodelSuite) TestFind(c *gc.C) {
	s.setupOffers(c)
	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"admin/controller", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
kontroll:admin/controller.riak:
  access: admin
  endpoints:
    endpoint:
      interface: http
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
kontroll:admin/controller.varnish:
  access: admin
  endpoints:
    webcache:
      interface: varnish
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
`[1:])
}

func (s *crossmodelSuite) TestFindOtherModel(c *gc.C) {
	s.addOtherModelApplication(c)

	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"otheruser/othermodel", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	otherUser, err := s.State.User(names.NewUserTag("otheruser"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, fmt.Sprintf(`
kontroll:otheruser/othermodel.hosted-mysql:
  access: admin
  endpoints:
    database:
      interface: mysql
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    otheruser:
      display-name: %s
      access: admin
`, otherUser.DisplayName())[1:])
}

func (s *crossmodelSuite) TestFindAllModels(c *gc.C) {
	s.setupOffers(c)
	s.addOtherModelApplication(c)

	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommand(), "kontroll:", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	otherUser, err := s.State.User(names.NewUserTag("otheruser"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, fmt.Sprintf(`
kontroll:admin/controller.riak:
  access: admin
  endpoints:
    endpoint:
      interface: http
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
kontroll:admin/controller.varnish:
  access: admin
  endpoints:
    webcache:
      interface: varnish
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    everyone@external:
      access: read
kontroll:otheruser/othermodel.hosted-mysql:
  access: admin
  endpoints:
    database:
      interface: mysql
      role: provider
  users:
    admin:
      display-name: admin
      access: admin
    otheruser:
      display-name: %s
      access: admin
`, otherUser.DisplayName())[1:])
}

func (s *crossmodelSuite) TestAddRelationFromURL(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)
	ch = s.AddTestingCharm(c, "mysql")
	s.AddTestingApplication(c, "mysql", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(),
		"mysql:server", "hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = runJujuCommand(c, "add-relation", "wordpress", "admin/controller.hosted-mysql")
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
				Interface: "mysql"},
		},
	})
}

func (s *crossmodelSuite) assertAddRelationSameControllerSuccess(c *gc.C, otherModeluser string) {
	_, err := runJujuCommand(c, "add-relation", "-m", "admin/controller", "wordpress", otherModeluser+"/othermodel.hosted-mysql")
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
			Relation: charm.Relation{Name: "database",
				Role:      "provider",
				Interface: "mysql"},
		},
	})
}

func (s *crossmodelSuite) TestAddRelationSameControllerSameOwner(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)

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
	ch, err = jujutesting.PutCharm(otherModel, curl, repo, false, false)
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
	otherOwner := s.Factory.MakeUser(c, &factory.UserParams{Name: "otheruser", DisplayName: "Other"})
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
	ch, err := jujutesting.PutCharm(otherModel, curl, repo, false, false)
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

func (s *crossmodelSuite) runJujuCommndWithStdin(c *gc.C, stdin io.Reader, args ...string) {
	context := cmdtesting.Context(c)
	if stdin != nil {
		context.Stdin = stdin
	}
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	loggo.RemoveWriter("warning") // remove logger added by main command
	err := command.Run(context)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("stdout: %q; stderr: %q", context.Stdout, context.Stderr))
}

func (s *crossmodelSuite) changeUserPassword(c *gc.C, user, password string) {
	s.runJujuCommndWithStdin(c, strings.NewReader(password+"\n"+password+"\n"), "change-user-password", user)
}

func (s *crossmodelSuite) createTestUser(c *gc.C) {
	runJujuCommand(c, "add-user", "test")
	runJujuCommand(c, "grant", "test", "read", "controller")
	s.changeUserPassword(c, "test", "hunter2")
}

func (s *crossmodelSuite) loginTestUser(c *gc.C) {
	// logout "admin" first; we'll need to give it
	// a non-random password before we can do so.
	s.changeUserPassword(c, "admin", "hunter2")
	runJujuCommand(c, "logout")
	s.runJujuCommndWithStdin(c, strings.NewReader("hunter2\nhunter2\n"), "login", "-u", "test")
}

func (s *crossmodelSuite) loginAdminUser(c *gc.C) {
	runJujuCommand(c, "logout")
	s.runJujuCommndWithStdin(c, strings.NewReader("hunter2\nhunter2\n"), "login", "-u", "admin")
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionDenied(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)
	s.addOtherModelApplication(c)

	s.createTestUser(c)
	s.loginTestUser(c)
	context, err := runJujuCommand(c, "add-relation", "-m", "admin/controller", "wordpress", "otheruser/othermodel.hosted-mysql")
	c.Assert(err, gc.NotNil)
	c.Assert(cmdtesting.Stderr(context), jc.Contains, `application offer "otheruser/othermodel.hosted-mysql" not found`)
}

func (s *crossmodelSuite) TestAddRelationSameControllerPermissionAllowed(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", ch)
	s.addOtherModelApplication(c)

	s.createTestUser(c)

	// Users with consume permission to the offer can add relations.
	runJujuCommand(c, "grant", "test", "consume", "otheruser/othermodel.hosted-mysql")
	runJujuCommand(c, "grant", "test", "write", "admin/controller")

	s.loginTestUser(c)
	s.assertAddRelationSameControllerSuccess(c, "otheruser")
}

func (s *crossmodelSuite) TestFindOffersWithPermission(c *gc.C) {
	s.addOtherModelApplication(c)
	s.createTestUser(c)
	s.loginTestUser(c)
	ctx, err := cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"otheruser/othermodel", "--format", "yaml")
	c.Assert(err, gc.ErrorMatches, ".*no matching application offers found.*")

	s.loginAdminUser(c)
	_, err = cmdtesting.RunCommand(c, model.NewGrantCommand(), "test", "read", "otheruser/othermodel.hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)

	s.loginTestUser(c)
	ctx, err = cmdtesting.RunCommand(c, crossmodel.NewFindEndpointsCommand(),
		"otheruser/othermodel", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
kontroll:otheruser/othermodel.hosted-mysql:
  access: read
  endpoints:
    database:
      interface: mysql
      role: provider
  users:
    test:
      access: read
`[1:])
}

func (s *crossmodelSuite) assertOfferGrant(c *gc.C) {
	ch := s.AddTestingCharm(c, "riak")
	s.AddTestingApplication(c, "riakservice", ch)

	_, err := cmdtesting.RunCommand(c, crossmodel.NewOfferCommand(), "riakservice:endpoint", "riak")
	c.Assert(err, jc.ErrorIsNil)

	// Check the default access levels.
	offer, err := state.NewApplicationOffers(s.State).ApplicationOffer("riak")
	c.Assert(err, jc.ErrorIsNil)
	userTag := names.NewUserTag("everyone@external")
	access, err := s.State.GetOfferAccess(offer.OfferUUID, userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)

	// Grant consume access.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "bob"})
	_, err = cmdtesting.RunCommand(c, model.NewGrantCommand(), "bob", "consume", "admin/controller.riak")
	c.Assert(err, jc.ErrorIsNil)
	access, err = s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("bob"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ConsumeAccess)

}

func (s *crossmodelSuite) TestOfferGrant(c *gc.C) {
	s.assertOfferGrant(c)
}

func (s *crossmodelSuite) TestOfferRevoke(c *gc.C) {
	s.assertOfferGrant(c)

	// Revoke consume access.
	_, err := cmdtesting.RunCommand(c, model.NewRevokeCommand(), "bob", "consume", "admin/controller.riak")
	c.Assert(err, jc.ErrorIsNil)
	offer, err := state.NewApplicationOffers(s.State).ApplicationOffer("riak")
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.GetOfferAccess(offer.OfferUUID, names.NewUserTag("bob"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.ReadAccess)
}

func (s *crossmodelSuite) TestConsumeWithPermission(c *gc.C) {
	s.addOtherModelApplication(c)
	s.createTestUser(c)
	s.loginTestUser(c)
	ctx, err := cmdtesting.RunCommand(c, application.NewConsumeCommand(),
		"-m", "admin/controller", "otheruser/othermodel.hosted-mysql")
	c.Assert(err, gc.ErrorMatches, `application offer "otheruser/othermodel.hosted-mysql" not found`)

	s.loginAdminUser(c)
	_, err = cmdtesting.RunCommand(c, model.NewGrantCommand(), "test", "consume", "otheruser/othermodel.hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmdtesting.RunCommand(c, model.NewGrantCommand(), "test", "write", "admin/controller")
	c.Assert(err, jc.ErrorIsNil)

	s.loginTestUser(c)
	ctx, err = cmdtesting.RunCommand(c, application.NewConsumeCommand(),
		"-m", "admin/controller", "otheruser/othermodel.hosted-mysql", "othermysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, `
Added otheruser/othermodel.hosted-mysql as othermysql
`[1:])
}

func (s *crossmodelSuite) TestRemoveSaas(c *gc.C) {
	s.addOtherModelApplication(c)
	_, err := cmdtesting.RunCommand(c, application.NewConsumeCommand(),
		"otheruser/othermodel.hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, application.NewRemoveSaasCommand(),
		"-m", "admin/controller", "hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)

	// A second time it's no longer there.
	ctx, err := cmdtesting.RunCommand(c, application.NewRemoveSaasCommand(),
		"-m", "admin/controller", "hosted-mysql")
	c.Check(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, `
removing SAAS application hosted-mysql failed: remote application "hosted-mysql" not found
`[1:])
}
