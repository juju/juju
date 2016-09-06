// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
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
		User: "bob@local",
	}
	s.store.Models = map[string]*jujuclient.ControllerModels{
		controllerName: {
			Models: map[string]jujuclient.ModelDetails{
				"bob@local/foo":    jujuclient.ModelDetails{fooModelUUID},
				"bob@local/bar":    jujuclient.ModelDetails{barModelUUID},
				"bob@local/baz":    jujuclient.ModelDetails{bazModelUUID},
				"bob@local/model1": jujuclient.ModelDetails{model1ModelUUID},
				"bob@local/model2": jujuclient.ModelDetails{model2ModelUUID},
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
	s.fake.err = &params.Error{Code: params.CodeOperationBlocked}
	_, err := s.run(c, "sam", "read", "foo")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	c.Check(c.GetTestLog(), jc.Contains, "To enable changes")
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
