// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type grantRevokeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake       *fakeGrantRevokeAPI
	cmdFactory func(*fakeGrantRevokeAPI) cmd.Command
	store      *jujuclienttesting.MemStore
}

const (
	fooModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee3"
	barModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee4"
	bazModelUUID    = "0701e916-3274-46e4-bd12-c31aff89cee5"
	model1ModelUUID = "0701e916-3274-46e4-bd12-c31aff89cee6"
	model2ModelUUID = "0701e916-3274-46e4-bd12-c31aff89cee7"
)

func (s *grantRevokeSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeGrantRevokeAPI{}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
	s.store.Models = map[string]*jujuclient.ControllerModels{
		controllerName: {
			Models: map[string]jujuclient.ModelDetails{
				"bob/foo":    jujuclient.ModelDetails{fooModelUUID},
				"bob/bar":    jujuclient.ModelDetails{barModelUUID},
				"bob/baz":    jujuclient.ModelDetails{bazModelUUID},
				"bob/model1": jujuclient.ModelDetails{model1ModelUUID},
				"bob/model2": jujuclient.ModelDetails{model2ModelUUID},
			},
		},
	}
}

func (s *grantRevokeSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := s.cmdFactory(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *grantRevokeSuite) TestPassesValues(c *gc.C) {
	user := "sam"
	models := []string{fooModelUUID, barModelUUID, bazModelUUID}
	_, err := s.run(c, "sam", "read", "foo", "bar", "baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.user, jc.DeepEquals, user)
	c.Assert(s.fake.modelUUIDs, jc.DeepEquals, models)
	c.Assert(s.fake.access, gc.Equals, "read")
}

func (s *grantRevokeSuite) TestAccess(c *gc.C) {
	sam := "sam"
	_, err := s.run(c, "sam", "write", "model1", "model2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.user, jc.DeepEquals, sam)
	c.Assert(s.fake.modelUUIDs, jc.DeepEquals, []string{model1ModelUUID, model2ModelUUID})
	c.Assert(s.fake.access, gc.Equals, "write")
}

func (s *grantRevokeSuite) TestBlockGrant(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockGrant")
	_, err := s.run(c, "sam", "read", "foo")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockGrant.*")
}

type grantSuite struct {
	grantRevokeSuite
}

var _ = gc.Suite(&grantSuite{})

func (s *grantSuite) SetUpTest(c *gc.C) {
	s.grantRevokeSuite.SetUpTest(c)
	s.cmdFactory = func(fake *fakeGrantRevokeAPI) cmd.Command {
		c, _ := model.NewGrantCommandForTest(fake, s.store)
		return c
	}
}

func (s *grantSuite) TestInit(c *gc.C) {
	wrappedCmd, grantCmd := model.NewGrantCommandForTest(s.fake, s.store)
	err := testing.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no user specified")

	err = testing.InitCommand(wrappedCmd, []string{"bob", "read", "model1", "model2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(grantCmd.User, gc.Equals, "bob")
	c.Assert(grantCmd.ModelNames, jc.DeepEquals, []string{"model1", "model2"})

	err = testing.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, `no user specified`)
}

// TestInitGrantAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to grant the AddModel permission.
func (s *grantSuite) TestInitGrantAddModel(c *gc.C) {
	wrappedCmd, grantCmd := model.NewGrantCommandForTest(s.fake, s.store)
	// The documented case, add-model.
	err := testing.InitCommand(wrappedCmd, []string{"bob", "add-model"})
	c.Check(err, jc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = testing.InitCommand(wrappedCmd, []string{"bob", "addmodel"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(grantCmd.Access, gc.Equals, "add-model")
}

type revokeSuite struct {
	grantRevokeSuite
}

var _ = gc.Suite(&revokeSuite{})

func (s *revokeSuite) SetUpTest(c *gc.C) {
	s.grantRevokeSuite.SetUpTest(c)
	s.cmdFactory = func(fake *fakeGrantRevokeAPI) cmd.Command {
		c, _ := model.NewRevokeCommandForTest(fake, s.store)
		return c
	}
}

func (s *revokeSuite) TestInit(c *gc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCommandForTest(s.fake, s.store)
	err := testing.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no user specified")

	err = testing.InitCommand(wrappedCmd, []string{"bob", "read", "model1", "model2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(revokeCmd.User, gc.Equals, "bob")
	c.Assert(revokeCmd.ModelNames, jc.DeepEquals, []string{"model1", "model2"})

	err = testing.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, `no user specified`)

}

// TestInitRevokeAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to revoke the AddModel permission.
func (s *grantSuite) TestInitRevokeAddModel(c *gc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCommandForTest(s.fake, s.store)
	// The documented case, add-model.
	err := testing.InitCommand(wrappedCmd, []string{"bob", "add-model"})
	c.Check(err, jc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = testing.InitCommand(wrappedCmd, []string{"bob", "addmodel"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(revokeCmd.Access, gc.Equals, "add-model")
}

func (s *grantSuite) TestModelAccessForController(c *gc.C) {
	wrappedCmd, _ := model.NewRevokeCommandForTest(s.fake, s.store)
	err := testing.InitCommand(wrappedCmd, []string{"bob", "write"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, gc.Matches, `You have specified a model access permission "write".*`)
}

func (s *grantSuite) TestControllerAccessForModel(c *gc.C) {
	wrappedCmd, _ := model.NewRevokeCommandForTest(s.fake, s.store)
	err := testing.InitCommand(wrappedCmd, []string{"bob", "superuser", "default"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, gc.Matches, `You have specified a controller access permission "superuser".*`)
}

type fakeGrantRevokeAPI struct {
	err        error
	user       string
	access     string
	modelUUIDs []string
}

func (f *fakeGrantRevokeAPI) Close() error { return nil }

func (f *fakeGrantRevokeAPI) GrantModel(user, access string, modelUUIDs ...string) error {
	return f.fake(user, access, modelUUIDs...)
}

func (f *fakeGrantRevokeAPI) RevokeModel(user, access string, modelUUIDs ...string) error {
	return f.fake(user, access, modelUUIDs...)
}

func (f *fakeGrantRevokeAPI) fake(user, access string, modelUUIDs ...string) error {
	f.user = user
	f.access = access
	f.modelUUIDs = modelUUIDs
	return f.err
}
