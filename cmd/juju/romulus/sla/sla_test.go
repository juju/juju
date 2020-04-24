// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sla_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	slawire "github.com/juju/romulus/wireformat/sla"
	"github.com/juju/testing"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/juju/romulus/sla"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&supportCommandSuite{})

type supportCommandSuite struct {
	jujutesting.CleanupSuite
	mockAPI       *mockapi
	mockSLAClient *mockSlaClient
	fakeAPIRoot   *fakeAPIConnection
	charmURL      string
	modelUUID     string
}

func (s *supportCommandSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	mockAPI, err := newMockAPI()
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = mockAPI
	s.mockSLAClient = &mockSlaClient{}
	s.modelUUID = utils.MustNewUUID().String()
	s.fakeAPIRoot = &fakeAPIConnection{model: s.modelUUID}
	s.PatchValue(sla.NewJujuClientStore, s.newMockClientStore)
	s.PatchValue(&rcmd.GetMeteringURLForModelCmd, func(c *modelcmd.ModelCommandBase) (string, error) {
		return "http://example.com", nil
	})
}

func (s *supportCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := sla.NewSLACommandForTest(s.fakeAPIRoot, s.mockSLAClient, s.mockAPI)
	command.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, command, args...)
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {}},
	}
	return store
}

func (s supportCommandSuite) TestSupportCommand(c *gc.C) {
	tests := []struct {
		about    string
		level    string
		budget   string
		err      string
		apiErr   error
		apiCalls []testing.StubCall
	}{{
		about: "all is well",
		level: "essential",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.modelUUID,
				"essential",
				"",
			},
		}},
	}, {
		about:  "all is well with budget",
		level:  "essential",
		budget: "personal:10",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.modelUUID,
				"essential",
				"personal:10",
			},
		}},
	},
	}
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		s.mockAPI.ResetCalls()
		if test.apiErr != nil {
			s.mockAPI.SetErrors(test.apiErr)
		}
		ctx, err := s.run(c, test.level, "--budget", test.budget)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 1)
			s.mockAPI.CheckCalls(c, test.apiCalls)
			c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `Model	SLA      	       Message
c:m1 	essential	a test message
`)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *supportCommandSuite) TestDiplayCurrentLevel(c *gc.C) {
	tests := []struct {
		format         string
		expectedOutput string
	}{{
		expectedOutput: `Model	SLA       	Message
c:m1 	mock-level	       
`,
	}, {
		format: "tabular",
		expectedOutput: `Model	SLA       	Message
c:m1 	mock-level	       
`,
	}, {
		format: "json",
		expectedOutput: `{"model":"c:m1","sla":"mock-level"}
`,
	}, {
		format: "yaml",
		expectedOutput: `model: c:m1
sla: mock-level
`,
	},
	}

	for i, test := range tests {
		c.Logf("running test %d", i)
		args := []string{}
		if test.format != "" {
			args = append(args, "--format", test.format)
		}
		ctx, err := s.run(c, args...)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("%s", errors.Details(err)))
		c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, test.expectedOutput)
	}
}

func newMockAPI() (*mockapi, error) {
	kp, err := bakery.GenerateKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc := bakery.NewOven(bakery.OvenParams{
		Location: "omnibus",
		Key:      kp,
	})
	return &mockapi{
		service: svc,
	}, nil
}

type mockapi struct {
	testing.Stub

	service  *bakery.Oven
	macaroon *macaroon.Macaroon
}

func (m *mockapi) Authorize(modelUUID, supportLevel, budget string) (*slawire.SLAResponse, error) {
	err := m.NextErr()
	if err != nil {
		return nil, errors.Trace(err)
	}
	m.AddCall("Authorize", modelUUID, supportLevel, budget)
	macaroon, err := m.service.NewMacaroon(context.Background(), bakery.LatestVersion, []checkers.Caveat{
		checkers.DeclaredCaveat("environment", modelUUID),
	}, bakery.NoOp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	m.macaroon = macaroon.M()
	return &slawire.SLAResponse{
		Credentials: m.macaroon,
		Owner:       "bob",
		Message:     "a test message",
	}, nil
}

type mockSlaClient struct {
	testing.Stub
}

func (m *mockSlaClient) SetSLALevel(level, owner string, creds []byte) error {
	m.AddCall("SetSLALevel", level, owner, creds)
	return nil
}
func (m *mockSlaClient) SLALevel() (string, error) {
	m.AddCall("SLALevel")
	return "mock-level", nil
}

type fakeAPIConnection struct {
	api.Connection
	model string
}

func (f *fakeAPIConnection) BestFacadeVersion(facade string) int {
	return 2
}

func (f *fakeAPIConnection) ModelTag() (names.ModelTag, bool) {
	return names.NewModelTag(f.model), false
}

type mockClientStore struct {
	jujuclient.ClientStore
	modelUUID string
}

func (s *supportCommandSuite) newMockClientStore() jujuclient.ClientStore {
	return &mockClientStore{modelUUID: s.modelUUID}
}

func (s *mockClientStore) AllControllers() (map[string]jujuclient.ControllerDetails, error) {
	return map[string]jujuclient.ControllerDetails{
		"c": {},
	}, nil
}

func (s *mockClientStore) AllModels(controllerName string) (map[string]jujuclient.ModelDetails, error) {
	return map[string]jujuclient.ModelDetails{
		"m1": {ModelUUID: s.modelUUID},
		"m2": {ModelUUID: "uuid2"},
		"m3": {ModelUUID: "uuid3"},
	}, nil
}
