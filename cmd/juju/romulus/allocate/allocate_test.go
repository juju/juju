// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package allocate_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/romulus/allocate"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&allocateSuite{})

type allocateSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	stub    *testing.Stub
	mockAPI *mockapi
	store   jujuclient.ClientStore
}

func (s *allocateSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = &jujuclient.MemStore{
		Controllers: map[string]jujuclient.ControllerDetails{
			"controller": {},
		},
		Models: map[string]*jujuclient.ControllerModels{
			"controller": {
				Models: map[string]jujuclient.ModelDetails{
					"admin/model": {"model-uuid"},
				},
				CurrentModel: "admin/model",
			},
		},
		Accounts: map[string]jujuclient.AccountDetails{
			"controller": {
				User: "admin",
			},
		},
	}
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
}

func (s *allocateSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	updateAlloc := allocate.NewAllocateCommandForTest(s.mockAPI, s.store)
	a := []string{"-m", "controller:model"}
	a = append(a, args...)
	return cmdtesting.RunCommand(c, updateAlloc, a...)
}

func (s *allocateSuite) TestUpdateAllocation(c *gc.C) {
	s.mockAPI.resp = "allocation set to 5"
	ctx, err := s.run(c, "5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "allocation set to 5\n")
	s.mockAPI.CheckCall(c, 0, "UpdateAllocation", "model-uuid", "5")
}

func (s *allocateSuite) TestUpdateAllocationAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))
	_, err := s.run(c, "5")
	c.Assert(err, gc.ErrorMatches, "failed to update the allocation: something failed")
	s.mockAPI.CheckCall(c, 0, "UpdateAllocation", "model-uuid", "5")
}

func (s *allocateSuite) TestUpdateAllocationErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{
		{
			about:         "value needs to be a number",
			args:          []string{"badvalue"},
			expectedError: "value needs to be a whole number",
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

func (api *mockapi) UpdateAllocation(modelUUID, value string) (string, error) {
	api.MethodCall(api, "UpdateAllocation", modelUUID, value)
	return api.resp, api.NextErr()
}
