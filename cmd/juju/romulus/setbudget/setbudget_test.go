// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setbudget_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/setbudget"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&setBudgetSuite{})

type setBudgetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
}

func (s *setBudgetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(setbudget.NewAPIClient, setbudget.APIClientFnc(s.mockAPI))
}

func (s *setBudgetSuite) TestSetBudget(c *gc.C) {
	s.mockAPI.resp = "name budget set to 5"
	set := setbudget.NewSetBudgetCommand()
	ctx, err := cmdtesting.RunCommand(c, set, "name", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "name budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "SetBudget", "name", "5")
}

func (s *setBudgetSuite) TestSetBudgetAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))
	set := setbudget.NewSetBudgetCommand()
	_, err := cmdtesting.RunCommand(c, set, "name", "5")
	c.Assert(err, gc.ErrorMatches, "failed to set the budget: something failed")
	s.mockAPI.CheckCall(c, 0, "SetBudget", "name", "5")
}

func (s *setBudgetSuite) TestSetBudgetErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "value needs to be a number",
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
		s.stub.SetErrors(errors.New(test.expectedError))
		defer s.mockAPI.ResetCalls()
		set := setbudget.NewSetBudgetCommand()
		_, err := cmdtesting.RunCommand(c, set, test.args...)
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

func (api *mockapi) SetBudget(name, value string) (string, error) {
	api.MethodCall(api, "SetBudget", name, value)
	return api.resp, api.NextErr()
}
