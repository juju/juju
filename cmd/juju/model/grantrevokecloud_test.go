// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"strings"

	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type grantRevokeCloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fakeCloudAPI *fakeCloudGrantRevokeAPI
	cmdFactory   func(*fakeCloudGrantRevokeAPI) cmd.Command
	store        *jujuclient.MemStore
}

func (s *grantRevokeCloudSuite) SetUpTest(c *tc.C) {
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

func (s *grantRevokeCloudSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	command := s.cmdFactory(s.fakeCloudAPI)
	return cmdtesting.RunCommand(c, command, args...)
}

func (s *grantRevokeCloudSuite) TestAccess(c *tc.C) {
	sam := "sam"
	_, err := s.run(c, "sam", "add-model", "cloud1", "cloud2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fakeCloudAPI.user, tc.DeepEquals, sam)
	c.Assert(s.fakeCloudAPI.clouds, tc.DeepEquals, []string{"cloud1", "cloud2"})
	c.Assert(s.fakeCloudAPI.access, tc.Equals, "add-model")
}

func (s *grantRevokeCloudSuite) TestBlockGrant(c *tc.C) {
	s.fakeCloudAPI.err = apiservererrors.OperationBlockedError("TestBlockGrant")
	_, err := s.run(c, "sam", "admin", "foo", "cloud")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockGrant.*")
}

type grantCloudSuite struct {
	grantRevokeCloudSuite
}

var _ = tc.Suite(&grantCloudSuite{})

func (s *grantCloudSuite) SetUpTest(c *tc.C) {
	s.grantRevokeCloudSuite.SetUpTest(c)
	s.cmdFactory = func(fakeCloudAPI *fakeCloudGrantRevokeAPI) cmd.Command {
		c, _ := model.NewGrantCloudCommandForTest(fakeCloudAPI, s.store)
		return c
	}
}

// TestInitGrantAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to grant the AddModel permission.
func (s *grantCloudSuite) TestInitGrantAddModel(c *tc.C) {
	wrappedCmd, grantCmd := model.NewGrantCloudCommandForTest(nil, s.store)
	// The documented case, add-model.
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud"})
	c.Check(err, tc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "addmodel", "cloud"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(grantCmd.Access, tc.Equals, "add-model")
}

type revokeCloudSuite struct {
	grantRevokeCloudSuite
}

var _ = tc.Suite(&revokeCloudSuite{})

func (s *revokeCloudSuite) SetUpTest(c *tc.C) {
	s.grantRevokeCloudSuite.SetUpTest(c)
	s.cmdFactory = func(fakeCloudAPI *fakeCloudGrantRevokeAPI) cmd.Command {
		c, _ := model.NewRevokeCloudCommandForTest(fakeCloudAPI, s.store)
		return c
	}
}

func (s *revokeCloudSuite) TestInit(c *tc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCloudCommandForTest(nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, "no user specified")

	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud1", "cloud2"})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(revokeCmd.User, tc.Equals, "bob")
	c.Assert(revokeCmd.Clouds, tc.DeepEquals, []string{"cloud1", "cloud2"})

	err = cmdtesting.InitCommand(wrappedCmd, []string{})
	c.Assert(err, tc.ErrorMatches, `no user specified`)

}

// TestInitRevokeAddModel checks that both the documented 'add-model' access and
// the backwards-compatible 'addmodel' work to revoke the AddModel permission.
func (s *grantCloudSuite) TestInitRevokeAddModel(c *tc.C) {
	wrappedCmd, revokeCmd := model.NewRevokeCloudCommandForTest(nil, s.store)
	// The documented case, add-model.
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "add-model", "cloud"})
	c.Check(err, tc.ErrorIsNil)

	// The backwards-compatible case, addmodel.
	err = cmdtesting.InitCommand(wrappedCmd, []string{"bob", "addmodel", "cloud"})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(revokeCmd.Access, tc.Equals, "add-model")
}

func (s *grantCloudSuite) TestWrongAccess(c *tc.C) {
	wrappedCmd, _ := model.NewRevokeCloudCommandForTest(nil, s.store)
	err := cmdtesting.InitCommand(wrappedCmd, []string{"bob", "write", "cloud"})
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Check(msg, tc.Matches, `"write" cloud access not valid`)
}

type fakeCloudGrantRevokeAPI struct {
	err    error
	user   string
	access string
	clouds []string
}

func (f *fakeCloudGrantRevokeAPI) Close() error { return nil }

func (f *fakeCloudGrantRevokeAPI) GrantCloud(ctx context.Context, user, access string, clouds ...string) error {
	return f.fake(user, access, clouds...)
}

func (f *fakeCloudGrantRevokeAPI) RevokeCloud(ctx context.Context, user, access string, clouds ...string) error {
	return f.fake(user, access, clouds...)
}

func (f *fakeCloudGrantRevokeAPI) fake(user, access string, clouds ...string) error {
	f.user = user
	f.access = access
	f.clouds = clouds
	return f.err
}
