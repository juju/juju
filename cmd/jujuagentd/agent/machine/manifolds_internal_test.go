// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"net/url"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type manifoldInternalSuite struct{}

func TestManifoldInternalSuite(t *stdtesting.T) {
	tc.Run(t, &manifoldInternalSuite{})
}

// mockAgentConn is an API connection double that records facade calls so
// the connect behaviour wired by newAPICallerConnection can be asserted.
// It embeds the api.Connection interface so only the methods the connect
// paths touch need overriding.
type mockAgentConn struct {
	stub *testhelpers.Stub
	api.Connection
}

func (conn *mockAgentConn) Addr() *url.URL {
	return &url.URL{Scheme: "wss", Host: "testing.invalid"}
}

func (*mockAgentConn) BestFacadeVersion(string) int { return 1 }

func (conn *mockAgentConn) APICall(_ context.Context, objType string, _ int, _, request string, _, response any) error {
	conn.stub.AddCall("APICall", objType, request)
	if err := conn.stub.NextErr(); err != nil {
		return err
	}
	switch r := response.(type) {
	case *params.AgentGetEntitiesResults:
		r.Entities = []params.AgentGetEntitiesResult{{Life: "alive"}}
	case *params.ErrorResults:
		r.Results = make([]params.ErrorResult, 1)
	}
	return nil
}

// connectAgent is an agent double whose loaded configuration carries the
// supplied tag and credentials.
type connectAgent struct {
	agent.Agent
	conf connectConfig
}

func (a *connectAgent) CurrentConfig() agent.Config { return &a.conf }

type connectConfig struct {
	agent.Config
	tag         names.Tag
	password    string
	apiPassword string
}

func (c *connectConfig) Tag() names.Tag { return c.tag }

func (c *connectConfig) OldPassword() string { return c.password }

func (c *connectConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		CACert:   "mock-ca-cert",
		Tag:      c.tag,
		Password: c.apiPassword,
		ModelTag: names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d"),
	}, true
}

// TestNewAPICallerConnectionSelection proves the machine agent's API
// connection behaviour is selected from the loaded configuration tag,
// never from the process's machine tag: a configuration tagged
// controller-<id> (the K8s controller pod's machine agent, running as
// machine-<id> with a pre-stored controller-node credential) wires
// OnlyConnect, which opens a connection with no facade interaction and
// never rotates, while a normal machine configuration wires ScaryConnect,
// which checks the entity's life and resets its password.
func (*manifoldInternalSuite) TestNewAPICallerConnectionSelection(c *tc.C) {
	openWith := func(stub *testhelpers.Stub, usedPassword *string) api.OpenFunc {
		conn := &mockAgentConn{stub: stub}
		return func(_ context.Context, info *api.Info, _ api.DialOpts) (api.Connection, error) {
			if usedPassword != nil {
				*usedPassword = info.Password
			}
			stub.AddCall("apiOpen")
			if err := stub.NextErr(); err != nil {
				return nil, err
			}
			return conn, nil
		}
	}

	// A process tagged machine-0 whose loaded configuration is tagged
	// controller-0 wires OnlyConnect: the connection is opened with no
	// facade interaction at all.
	stub := &testhelpers.Stub{}
	controllerNodePassword := "controller-node-credential"
	var usedPassword string
	_, err := newAPICallerConnection(c.Context(),
		&connectAgent{conf: connectConfig{
			tag:      names.NewControllerAgentTag("0"),
			password: controllerNodePassword,
		}},
		openWith(stub, &usedPassword), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "apiOpen"}})
	c.Check(usedPassword, tc.Equals, controllerNodePassword)

	// A normal machine configuration wires ScaryConnect: the agent
	// entity's life is checked and its password reset through the
	// connection.
	stub = &testhelpers.Stub{}
	_, err = newAPICallerConnection(c.Context(),
		&connectAgent{conf: connectConfig{
			tag:         names.NewMachineTag("0"),
			apiPassword: "mock-password",
		}},
		openWith(stub, nil), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	stub.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "apiOpen"},
		{FuncName: "APICall", Args: []any{"Agent", "GetEntities"}},
		{FuncName: "APICall", Args: []any{"Agent", "SetPasswords"}},
	})
}
