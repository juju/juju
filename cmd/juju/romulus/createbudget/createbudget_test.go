// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package createbudget_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/createbudget"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&createBudgetSuite{})

type createBudgetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *createBudgetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(createbudget.NewAPIClient, createbudget.APIClientFnc(s.mockAPI))
}

func (s *createBudgetSuite) TestCreateBudget(c *gc.C) {
	s.mockAPI.resp = "name budget set to 5"
	createCmd := createbudget.NewCreateBudgetCommand()
	ctx, err := cmdtesting.RunCommand(c, createCmd, "name", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "name budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "CreateBudget", "name", "5")
}

func (s *createBudgetSuite) TestCreateBudgetAPIError(c *gc.C) {
	s.mockAPI.SetErrors(errors.New("something failed"))
	createCmd := createbudget.NewCreateBudgetCommand()
	_, err := cmdtesting.RunCommand(c, createCmd, "name", "5")
	c.Assert(err, gc.ErrorMatches, "failed to create the budget: something failed")
	s.mockAPI.CheckCall(c, 0, "CreateBudget", "name", "5")
}

func (s *createBudgetSuite) TestCreateBudgetErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "test value needs to be a number",
			args:          []string{"name", "badvalue"},
			expectedError: "budget value needs to be a whole number",
		},
		{
			about:         "value is missing",
			args:          []string{"name"},
			expectedError: "name and value required",
		},
		{
			about:         "no args",
			args:          []string{},
			expectedError: "name and value required",
		},
	}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		if test.expectedError != "" {
			s.mockAPI.SetErrors(errors.New(test.expectedError))
		}
		createCmd := createbudget.NewCreateBudgetCommand()
		_, err := cmdtesting.RunCommand(c, createCmd, test.args...)
		c.Assert(err, gc.ErrorMatches, test.expectedError)
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

func (api *mockapi) CreateBudget(name, value string) (string, error) {
	api.MethodCall(api, "CreateBudget", name, value)
	return api.resp, api.NextErr()
}
