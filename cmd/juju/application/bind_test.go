// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
)

type BindSuite struct {
	testing.IsolationSuite
	testing.Stub

	apiConnection     mockAPIConnection
	applicationClient mockApplicationBindClient
	spacesClient      mockSpacesClient
	cmd               cmd.Command
}

var _ = gc.Suite(&BindSuite{})

func (s *BindSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	// Create persistent cookies in a temporary location.
	cookieFile := filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookieFile)

	s.apiConnection = mockAPIConnection{
		bestFacadeVersion: 2,
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
	s.applicationClient = mockApplicationBindClient{}
	s.spacesClient = mockSpacesClient{
		spaceList: []params.Space{
			{Id: "0", Name: network.AlphaSpaceName},
			{Id: "1", Name: "sp1"},
			{Id: "4", Name: "sp4"},
			{Id: "5", Name: "testing"},
		},
	}

	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {ActiveBranch: model.GenerationMaster}},
	}
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.AddCall("OpenAPI")
		return &s.apiConnection, nil
	}

	s.cmd = NewBindCommandForTest(
		store,
		apiOpen,
		func(conn base.APICallCloser) ApplicationBindClient {
			s.AddCall("NewApplicationClient", conn)
			return &s.applicationClient
		},
		func(conn base.APICallCloser) SpacesAPI {
			s.AddCall("NewSpacesClient", conn)
			return &s.spacesClient
		},
	)
}

func (s *BindSuite) runBind(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.cmd, args...)
}

func (s *BindSuite) TestBind(c *gc.C) {
	s.setupAPIConnection(11)
	s.applicationClient.getResults = &params.ApplicationGetResults{
		EndpointBindings: map[string]string{
			"ep1": network.AlphaSpaceName,
			"ep2": "sp2",
		},
	}

	_, err := s.runBind(c, "foo", "ep1=sp1")
	c.Assert(err, jc.ErrorIsNil)
	s.spacesClient.CheckCallNames(c, "ListSpaces")
	s.applicationClient.CheckCallNames(c, "Get", "MergeBindings")
	s.applicationClient.CheckCall(c, 1, "MergeBindings", params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{
			{
				ApplicationTag: names.NewApplicationTag("foo").String(),
				Bindings: map[string]string{
					"ep1": "sp1",
					"ep2": "sp2",
				},
			},
		},
	})
}

func (s *BindSuite) TestBindWithNoBindings(c *gc.C) {
	s.setupAPIConnection(11)

	_, err := s.runBind(c, "foo")
	c.Assert(err, gc.ErrorMatches, "no bindings specified")
}

func (s *BindSuite) TestBindUnknownEndpoint(c *gc.C) {
	s.setupAPIConnection(11)
	s.applicationClient.getResults = &params.ApplicationGetResults{
		EndpointBindings: map[string]string{
			"ep1": network.AlphaSpaceName,
			"ep2": "sp2",
		},
	}

	_, err := s.runBind(c, "foo", "unknown=sp1")
	c.Assert(err, gc.ErrorMatches, `endpoint "unknown" not found`)
}

func (s *BindSuite) TestBindWithOlderController(c *gc.C) {
	s.setupAPIConnection(10)

	_, err := s.runBind(c, "foo", "unknown=sp1")
	c.Assert(err, gc.ErrorMatches, `changing application bindings is not supported by server version.*`)
}

func (s *BindSuite) setupAPIConnection(bestFacadeVersion int) {
	s.apiConnection = mockAPIConnection{
		bestFacadeVersion: bestFacadeVersion,
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
}

type mockApplicationBindClient struct {
	ApplicationBindClient
	testing.Stub

	getResults *params.ApplicationGetResults
}

func (m *mockApplicationBindClient) Get(generation string, app string) (*params.ApplicationGetResults, error) {
	m.MethodCall(m, "Get", generation, app)
	return m.getResults, m.NextErr()
}

func (m *mockApplicationBindClient) MergeBindings(p params.ApplicationMergeBindingsArgs) error {
	m.MethodCall(m, "MergeBindings", p)
	return m.NextErr()
}
