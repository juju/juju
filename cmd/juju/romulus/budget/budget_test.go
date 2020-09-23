// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package budget_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	cookiejar "github.com/juju/persistent-cookiejar"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/juju/romulus/budget"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&budgetSuite{})

type budgetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
	store   jujuclient.ClientStore
}

func (s *budgetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = &jujuclient.MemStore{
		CurrentControllerName: "controller",
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller":        {},
			"anothercontroller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/model": {ModelUUID: "model-uuid", ModelType: model.IAAS},
				},
				CurrentModel: "admin/model",
			},
			"anothercontroller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/somemodel": {ModelUUID: "another-model-uuid", ModelType: model.IAAS},
				},
				CurrentModel: "admin/somemodel",
			},
		},
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin",
			},
			"anothercontroller": {
				User: "admin",
			},
		},
		CookieJars: make(map[string]*cookiejar.Jar),
	}
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(&rcmd.GetMeteringURLForModelCmd, func(c *modelcmd.ModelCommandBase) (string, error) {
		return "http://example.com", nil
	})
}

func (s *budgetSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	updateBudget := budget.NewBudgetCommandForTest(s.mockAPI, s.store)
	return cmdtesting.RunCommand(c, updateBudget, args...)
}

func (s *budgetSuite) TestUpdateBudget(c *gc.C) {
	s.mockAPI.resp = "budget set to 5"
	ctx, err := s.run(c, "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateBudget", "model-uuid", "", "5")
}

func (s *budgetSuite) TestUpdateBudgetMoveToWallet(c *gc.C) {
	s.mockAPI.resp = "budget moved to wallet 'work', budget set to 5"
	ctx, err := s.run(c, "work:5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "budget moved to wallet 'work', budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateBudget", "model-uuid", "work", "5")
}

func (s *budgetSuite) TestUpdateBudgetByModelUUID(c *gc.C) {
	modelUUID := utils.MustNewUUID().String()
	s.mockAPI.resp = "budget set to 5"
	ctx, err := s.run(c, "--model-uuid", modelUUID, "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateBudget", modelUUID, "", "5")
}

func (s *budgetSuite) TestUpdateBudgetByModelName(c *gc.C) {
	s.mockAPI.resp = "budget set to 5"
	ctx, err := s.run(c, "--model", "anothercontroller:somemodel", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateBudget", "another-model-uuid", "", "5")
}

func (s *budgetSuite) TestUpdateBudgetInvalidModelUUID(c *gc.C) {
	ctx, err := s.run(c, "--model-uuid", "not-a-uuid", "5")
	c.Assert(err, gc.ErrorMatches, `provided model UUID "not-a-uuid" not valid`)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "")
	s.mockAPI.CheckNoCalls(c)
}

func (s *budgetSuite) TestUpdateBudgetAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))
	_, err := s.run(c, "5")
	c.Assert(err, gc.ErrorMatches, "failed to update the budget: something failed")
	s.mockAPI.CheckCall(c, 0, "UpdateBudget", "model-uuid", "", "5")
}

func (s *budgetSuite) TestUpdateBudgetErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "budget limit needs to be a number",
			args:          []string{"badvalue"},
			expectedError: "budget limit needs to be a whole number",
		}, {
			about:         "budget limit needs to be a number",
			args:          []string{"wallet:"},
			expectedError: "budget limit needs to be a whole number",
		}, {
			about:         "no args",
			args:          []string{},
			expectedError: "value required",
		},
	}
	for i, test := range tests {
		s.mockAPI.ResetCalls()
		c.Logf("test %d: %s", i, test.about)
		_, err := s.run(c, test.args...)
		c.Check(err, gc.ErrorMatches, test.expectedError)
		s.mockAPI.CheckNoCalls(c)
	}
}

func newMockAPI(s *testing.Stub) *mockapi {
	return &mockapi{Stub: s}
}

type mockapi struct {
	*testing.Stub
	resp string
}

func (api *mockapi) UpdateBudget(modelUUID, wallet, value string) (string, error) {
	api.MethodCall(api, "UpdateBudget", modelUUID, wallet, value)
	return api.resp, api.NextErr()
}
