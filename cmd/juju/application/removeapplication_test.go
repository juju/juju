// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	jutesting "github.com/juju/testing"
)

type RemoveServiceSuite struct {
	jujutesting.RepoSuite
	testing.CmdBlockHelper
	stub            *jutesting.Stub
	budgetAPIClient budgetAPIClient
}

var _ = gc.Suite(&RemoveServiceSuite{})

func (s *RemoveServiceSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
	s.stub = &jutesting.Stub{}
	s.budgetAPIClient = &mockBudgetAPIClient{Stub: s.stub}
	s.PatchValue(&getBudgetAPIClient, func(*httpbakery.Client) budgetAPIClient { return s.budgetAPIClient })
}

func runRemoveService(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, NewRemoveServiceCommand(), args...)
	return err
}

func (s *RemoveServiceSuite) setupTestService(c *gc.C) {
	// Destroy an application that exists.
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "riak")
	err := runDeploy(c, ch, "riak", "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RemoveServiceSuite) TestSuccess(c *gc.C) {
	s.setupTestService(c)
	err := runRemoveService(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
	s.stub.CheckNoCalls(c)
}

func (s *RemoveServiceSuite) TestRemoveLocalMetered(c *gc.C) {
	ch := testcharms.Repo.CharmArchivePath(s.CharmsPath, "metered")
	deploy := NewDeployCommand()
	_, err := testing.RunCommand(c, deploy, ch, "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	err = runRemoveService(c, "metered")
	c.Assert(err, jc.ErrorIsNil)
	riak, err := s.State.Application("metered")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Dying)
	s.stub.CheckNoCalls(c)
}

func (s *RemoveServiceSuite) TestBlockRemoveService(c *gc.C) {
	s.setupTestService(c)

	// block operation
	s.BlockRemoveObject(c, "TestBlockRemoveService")
	err := runRemoveService(c, "riak")
	s.AssertBlocked(c, err, ".*TestBlockRemoveService.*")
	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(riak.Life(), gc.Equals, state.Alive)
	s.stub.CheckNoCalls(c)
}

func (s *RemoveServiceSuite) TestFailure(c *gc.C) {
	// Destroy an application that does not exist.
	err := runRemoveService(c, "gargleblaster")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "gargleblaster" not found`,
		Code:    "not found",
	})
	s.stub.CheckNoCalls(c)
}

func (s *RemoveServiceSuite) TestInvalidArgs(c *gc.C) {
	err := runRemoveService(c)
	c.Assert(err, gc.ErrorMatches, `no application specified`)
	err = runRemoveService(c, "ping", "pong")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["pong"\]`)
	err = runRemoveService(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	s.stub.CheckNoCalls(c)
}

type RemoveCharmStoreCharmsSuite struct {
	charmStoreSuite
	stub            *jutesting.Stub
	ctx             *cmd.Context
	budgetAPIClient budgetAPIClient
}

var _ = gc.Suite(&RemoveCharmStoreCharmsSuite{})

func (s *RemoveCharmStoreCharmsSuite) SetUpTest(c *gc.C) {
	s.charmStoreSuite.SetUpTest(c)

	s.ctx = testing.Context(c)
	s.stub = &jutesting.Stub{}
	s.budgetAPIClient = &mockBudgetAPIClient{Stub: s.stub}
	s.PatchValue(&getBudgetAPIClient, func(*httpbakery.Client) budgetAPIClient { return s.budgetAPIClient })

	testcharms.UploadCharm(c, s.client, "cs:quantal/metered-1", "metered")
	deployCmd := &DeployCommand{}
	cmd := modelcmd.Wrap(deployCmd)
	deployCmd.NewAPIRoot = func() (DeployAPI, error) {
		apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &deployAPIAdapter{
			Connection:        apiRoot,
			apiClient:         &apiClient{Client: apiRoot.Client()},
			charmsClient:      &charmsClient{Client: charms.NewClient(apiRoot)},
			applicationClient: &applicationClient{application.NewClient(apiRoot)},
		}, nil
	}

	_, err := testing.RunCommand(c, cmd, "cs:quantal/metered-1")
	c.Assert(err, jc.ErrorIsNil)

}

func (s *RemoveCharmStoreCharmsSuite) TestRemoveAllocation(c *gc.C) {
	err := runRemoveService(c, "metered")
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []jutesting.StubCall{{
		"DeleteAllocation", []interface{}{testing.ModelTag.Id(), "metered"}}})
}

type mockBudgetAPIClient struct {
	*jutesting.Stub
}

// CreateAllocation implements apiClient.
func (c *mockBudgetAPIClient) CreateAllocation(budget, limit, model string, services []string) (string, error) {
	c.MethodCall(c, "CreateAllocation", budget, limit, model, services)
	return "Allocation created.", c.NextErr()
}

// DeleteAllocation implements apiClient.
func (c *mockBudgetAPIClient) DeleteAllocation(model, application string) (string, error) {
	c.MethodCall(c, "DeleteAllocation", model, application)
	return "Allocation removed.", c.NextErr()
}
