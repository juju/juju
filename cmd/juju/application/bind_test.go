// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type BindSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub

	apiConnection     mockAPIConnection
	applicationClient mockApplicationBindClient
	spacesClient      mockSpacesClient
	cmd               cmd.Command
}

func TestBindSuite(t *testing.T) {
	tc.Run(t, &BindSuite{})
}

func (s *BindSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	// Create persistent cookies in a temporary location.
	cookieFile := filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookieFile)

	s.apiConnection = mockAPIConnection{
		authTag: names.NewUserTag("testuser"),
		serverVersion: &semversion.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
	s.applicationClient = mockApplicationBindClient{}
	s.spacesClient = mockSpacesClient{
		spaceList: []params.Space{
			{Id: "0", Name: network.AlphaSpaceName.String()},
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
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {}},
	}
	store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "testuser",
	}
	apiOpen := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
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

func (s *BindSuite) runBind(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.cmd, args...)
}

func (s *BindSuite) TestBind(c *tc.C) {
	s.setupAPIConnection()
	s.applicationClient.getResults = &params.ApplicationGetResults{
		EndpointBindings: map[string]string{
			"ep1": network.AlphaSpaceName.String(),
			"ep2": "sp2",
		},
	}

	_, err := s.runBind(c, "foo", "ep1=sp1")
	c.Assert(err, tc.ErrorIsNil)
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

func (s *BindSuite) TestBindWithNoBindings(c *tc.C) {
	s.setupAPIConnection()

	_, err := s.runBind(c, "foo")
	c.Assert(err, tc.ErrorMatches, "no bindings specified")
}

func (s *BindSuite) TestBindUnknownEndpoint(c *tc.C) {
	s.setupAPIConnection()
	s.applicationClient.getResults = &params.ApplicationGetResults{
		EndpointBindings: map[string]string{
			"ep1": network.AlphaSpaceName.String(),
			"ep2": "sp2",
		},
	}

	_, err := s.runBind(c, "foo", "unknown=sp1")
	c.Assert(err, tc.ErrorMatches, `endpoint "unknown" not found`)
}

func (s *BindSuite) setupAPIConnection() {
	s.apiConnection = mockAPIConnection{
		authTag: names.NewUserTag("testuser"),
		serverVersion: &semversion.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
}

type mockApplicationBindClient struct {
	ApplicationBindClient
	testhelpers.Stub

	getResults *params.ApplicationGetResults
}

func (m *mockApplicationBindClient) Get(ctx context.Context, app string) (*params.ApplicationGetResults, error) {
	m.MethodCall(m, "Get", "", app)
	return m.getResults, m.NextErr()
}

func (m *mockApplicationBindClient) MergeBindings(ctx context.Context, p params.ApplicationMergeBindingsArgs) error {
	m.MethodCall(m, "MergeBindings", p)
	return m.NextErr()
}
