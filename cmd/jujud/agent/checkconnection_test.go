// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
)

type checkConnectionSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&checkConnectionSuite{})

func (s *checkConnectionSuite) TestInitChecksTag(c *tc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(nil, nil)
	err := cmd.Init(nil)
	c.Assert(err, tc.ErrorMatches, "agent-name argument is required")
	err = cmd.Init([]string{"aloy"})
	c.Assert(err, tc.ErrorMatches, `agent-name: "aloy" is not a valid tag`)
	err = cmd.Init([]string{"user-eleuthia"})
	c.Assert(err, tc.ErrorMatches, `agent-name must be a machine or unit tag`)
	err = cmd.Init([]string{"unit-demeter-0", "minerva"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["minerva"\]`)
}

func (s *checkConnectionSuite) TestRunComplainsAboutConnectionErrors(c *tc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(newAgentConf(),
		func(_ context.Context, a agent.Agent) (io.Closer, error) {
			return nil, errors.Errorf("hartz-timor swarm detected")
		})
	c.Assert(cmd.Init([]string{"unit-artemis-5"}), tc.ErrorIsNil)
	err := cmd.Run(nil)
	c.Assert(err, tc.ErrorMatches, "checking connection for unit-artemis-5: hartz-timor swarm detected")
}

func (s *checkConnectionSuite) TestRunClosesConnection(c *tc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(newAgentConf(),
		func(_ context.Context, a agent.Agent) (io.Closer, error) {
			return &mockConnection{}, nil
		})
	c.Assert(cmd.Init([]string{"unit-artemis-5"}), tc.ErrorIsNil)
	err := cmd.Run(nil)
	c.Assert(err, tc.ErrorMatches, "closing connection for unit-artemis-5: seal integrity check failed")
}

func newAgentConf() *mockAgentConf {
	return &mockAgentConf{stub: &testing.Stub{}}
}

type mockAgentConf struct {
	agentconf.AgentConf
	stub *testing.Stub
}

func (c *mockAgentConf) ReadConfig(tag string) error {
	c.stub.AddCall("ReadConfig", tag)
	return c.stub.NextErr()
}

type mockConnection struct{}

func (c *mockConnection) Close() error {
	return errors.Errorf("seal integrity check failed")
}
