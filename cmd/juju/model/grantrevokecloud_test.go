// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type grantRevokeCloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeCloudAPI *fakeCloudGrantRevokeAPI
	cmdFactory   func(*fakeCloudGrantRevokeAPI) cmd.Command
	store        *jujuclient.MemStore
}

func (s *grantRevokeCloudSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fakeCloudAPI = &fakeCloudGrantRevokeAPI{}

	// Set up the current controller, and write just enough info
	// so we don't try to refresh
	controllerName := "test-master"

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = controllerName
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{}
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
	s.store.Models = map[string]*jujuclient.ControllerModels{
		controllerName: {
			Models: map[string]jujuclient.ModelDetails{
				"bob/foo": {ModelUUID: fooModelUUID, ModelType: coremodel.IAAS},
			},
		},
	}
}

func (s *grantRevokeCloudSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := s.cmdFactory(s.fakeCloudAPI)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *grantRevokeCloudSuite) TestAccess(c *gc.C) {
	sam := "sam"
	_, err := s.run(c, "sam", "add-model", "cloud1", "cloud2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakeCloudAPI.user, jc.DeepEquals, sam)
	c.Assert(s.fakeCloudAPI.clouds, jc.DeepEquals, []string{"cloud1", "cloud2"})
	c.Assert(s.fakeCloudAPI.access, gc.Equals, "add-model")
}

func (s *grantRevokeCloudSuite) TestBlockGrant(c *gc.C) {
	s.fakeCloudAPI.err = common.OperationBlockedError("TestBlockGrant")
	_, err := s.run(c, "sam", "admin", "foo", "cloud")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockGrant.*")
}

type grantCloudSuite struct {
	grantRevokeCloudSuite
}

var _ = gc.Suite(&grantCloudSuite{})

func (s *grantCloudSuite) SetUpTest(c *gc.C) {
	s.grantRevokeCloudSuite.SetUpTest(c)
	s.cmdFactory = func(fakeCloudAPI *fakeCloudGrantRevokeAPI) cmd.Command {
		c, _ := model.NewGrantCloudCommandForTest(fakeCloudAPI, s.store)
		return c
	}
}

// TestInitGrantAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to grant the AddModel permission.
func (s *grantCloudSuite) TestInitGrantAddModel(c *gc.C) {
	wrappedCmd, grantCmd := model.NewGrantCloudCommandForTest(nil, s.store)
	// The documented case, add-model.
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud"})
	c.Check(err, jc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "addmodel", "cloud"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(grantCmd.Access, gc.Equals, "add-model")
}

type revokeCloudSuite struct {
	grantRevokeCloudSuite
}

var _ = gc.Suite(&revokeCloudSuite{})

func (s *revokeCloudSuite) SetUpTest(c *gc.C) {
	s.grantRevokeCloudSuite.SetUpTest(c)
	s.cmdFactory = func(fakeCloudAPI *fakeCloudGrantRevokeAPI) cmd.Command {
		c, _ := model.NewRevokeCloudCommandForTest(fakeCloudAPI, s.store)
		return c
	}
}

func (s *revokeCloudSuite) TestInit(c *gc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCloudCommandForTest(nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no user specified")

	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud1", "cloud2"})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(revokeCmd.User, gc.Equals, "bob")
	c.Assert(revokeCmd.Clouds, jc.DeepEquals, []string{"cloud1", "cloud2"})

	err = cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, gc.ErrorMatches, `no user specified`)

}

// TestInitRevokeAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to revoke the AddModel permission.
func (s *grantCloudSuite) TestInitRevokeAddModel(c *gc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCloudCommandForTest(nil, s.store)
	// The documented case, add-model.
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud"})
	c.Check(err, jc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "addmodel", "cloud"})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(revokeCmd.Access, gc.Equals, "add-model")
}

func (s *grantCloudSuite) TestWrongAccess(c *gc.C) {
	wrappedCmd, _ := model.NewRevokeCloudCommandForTest(nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "write", "cloud"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, gc.Matches, `"write" cloud access not valid`)
}

type fakeCloudGrantRevokeAPI struct {
	err    error
	user   string
	access string
	clouds []string
}

func (f *fakeCloudGrantRevokeAPI) Close() error { return nil }

func (f *fakeCloudGrantRevokeAPI) GrantCloud(user, access string, clouds ...string) error {
	return f.fake(user, access, clouds...)
}

func (f *fakeCloudGrantRevokeAPI) RevokeCloud(user, access string, clouds ...string) error {
	return f.fake(user, access, clouds...)
}

func (f *fakeCloudGrantRevokeAPI) fake(user, access string, clouds ...string) error {
	f.user = user
	f.access = access
	f.clouds = clouds
	return f.err
}
