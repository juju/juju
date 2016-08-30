// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updateallocation_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/updateallocation"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&updateAllocationSuite{})

type updateAllocationSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
	store   jujuclient.ClientStore
}

func (s *updateAllocationSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = &jujuclienttesting.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin@local/model": {"model-uuid"},
				},
				CurrentModel: "admin@local/model",
			},
		},
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin@local",
			},
		},
	}
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
}

func (s *updateAllocationSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	updateAlloc := updateallocation.NewUpdateAllocateCommandForTest(s.mockAPI, s.store)
	a := []string{"-m", "controller:model"}
	a = append(a, args...)
	return cmdtesting.RunCommand(c, updateAlloc, a...)
}

func (s *updateAllocationSuite) TestUpdateAllocation(c *gc.C) {
	s.mockAPI.resp = "name budget set to 5"
	ctx, err := s.run(c, "name", "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "name budget set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateAllocation", "model-uuid", "name", "5")
}

func (s *updateAllocationSuite) TestUpdateAllocationAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))
	_, err := s.run(c, "name", "5")
	c.Assert(err, gc.ErrorMatches, "failed to update the allocation: something failed")
	s.mockAPI.CheckCall(c, 0, "UpdateAllocation", "model-uuid", "name", "5")
}

func (s *updateAllocationSuite) TestUpdateAllocationErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "value needs to be a number",
			args:          []string{"name", "badvalue"},
			expectedError: "value needs to be a whole number",
		},
		{
			about:         "value is missing",
			args:          []string{"name"},
			expectedError: "application and value required",
		},
		{
			about:         "no args",
			args:          []string{},
			expectedError: "application and value required",
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

func (api *mockapi) UpdateAllocation(modelUUID, name, value string) (string, error) {
	api.MethodCall(api, "UpdateAllocation", modelUUID, name, value)
	return api.resp, api.NextErr()
}
