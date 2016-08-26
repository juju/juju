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
	"github.com/juju/juju/jujuclient/jujuclienttesting"
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
	s.store = &jujuclienttesting.MemStore{
		CurrentControllerName: "controller",
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

func (s *allocateSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	alloc := allocate.NewAllocateCommandForTest(s.mockAPI, s.store)
	a := []string{"-m", "controller:model"}
	a = append(a, args...)
	return cmdtesting.RunCommand(c, alloc, a...)
}

func (s *allocateSuite) TestAllocate(c *gc.C) {
	s.mockAPI.resp = "allocation updated"
	ctx, err := s.run(c, "name:100", "db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "allocation updated\n")
	s.mockAPI.CheckCall(c, 0, "CreateAllocation", "name", "100", "model-uuid", []string{"db"})
}

func (s *allocateSuite) TestAllocateAPIError(c *gc.C) {
	s.stub.SetErrors(errors.New("something failed"))
	_, err := s.run(c, "name:100", "db")
	c.Assert(err, gc.ErrorMatches, "failed to create allocation: something failed")
	s.mockAPI.CheckCall(c, 0, "CreateAllocation", "name", "100", "model-uuid", []string{"db"})
}

func (s *allocateSuite) TestAllocateZero(c *gc.C) {
	s.mockAPI.resp = "allocation updated"
	_, err := s.run(c, "name:0", "db")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "CreateAllocation", "name", "0", "model-uuid", []string{"db"})
}

func (s *allocateSuite) TestAllocateModelUUID(c *gc.C) {
	s.mockAPI.resp = "allocation updated"
	_, err := s.run(c, "name:0", "--model-uuid", "30f7a9f2-220d-4268-b336-35e7daacae79", "db")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCall(c, 0, "CreateAllocation", "name", "0", "30f7a9f2-220d-4268-b336-35e7daacae79", []string{"db"})
}

func (s *allocateSuite) TestAllocateErrors(c *gc.C) {
	tests := []struct {
		about         string
		args          []string
		expectedError string
	}{{
		about:         "no args",
		args:          []string{},
		expectedError: "budget and application name required",
	}, {
		about:         "budget without allocation limit",
		args:          []string{"name", "db"},
		expectedError: `expected args in the form "budget:limit \[application ...\]": invalid budget specification, expecting <budget>:<limit>`,
	}, {
		about:         "application not specified",
		args:          []string{"name:100"},
		expectedError: "budget and application name required",
	}, {
		about:         "negative allocation limit",
		args:          []string{"name:-100", "db"},
		expectedError: `expected args in the form "budget:limit \[application ...\]": invalid budget specification, expecting <budget>:<limit>`,
	}, {
		about:         "non-numeric allocation limit",
		args:          []string{"name:abcd", "db"},
		expectedError: `expected args in the form "budget:limit \[application ...\]": invalid budget specification, expecting <budget>:<limit>`,
	}, {
		about:         "empty allocation limit",
		args:          []string{"name:", "db"},
		expectedError: `expected args in the form "budget:limit \[application ...\]": invalid budget specification, expecting <budget>:<limit>`,
	}, {
		about:         "invalid model UUID",
		args:          []string{"--model-uuid", "nope", "name:100", "db"},
		expectedError: `model UUID "nope" not valid`,
	}, {
		about:         "arguments in wrong order",
		args:          []string{"name:", "db:50"},
		expectedError: `expected args in the form "budget:limit \[application ...\]": invalid budget specification, expecting <budget>:<limit>`,
	}}
	for i, test := range tests {
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

func (api *mockapi) CreateAllocation(name, limit, modelUUID string, services []string) (string, error) {
	api.MethodCall(api, "CreateAllocation", name, limit, modelUUID, services)
	return api.resp, api.NextErr()
}
