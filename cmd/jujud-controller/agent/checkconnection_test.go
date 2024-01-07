// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	agentcmd "github.com/juju/juju/cmd/jujud-controller/agent"
	"github.com/juju/juju/cmd/jujud-controller/agent/agentconf"
)

type checkConnectionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&checkConnectionSuite{})

func (s *checkConnectionSuite) TestInitChecksTag(c *gc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(nil, nil)
	err := cmd.Init(nil)
	c.Assert(err, gc.ErrorMatches, "agent-name argument is required")
	err = cmd.Init([]string{"aloy"})
	c.Assert(err, gc.ErrorMatches, `agent-name: "aloy" is not a valid tag`)
	err = cmd.Init([]string{"user-eleuthia"})
	c.Assert(err, gc.ErrorMatches, `agent-name must be a machine or unit tag`)
	err = cmd.Init([]string{"unit-demeter-0", "minerva"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["minerva"\]`)
}

func (s *checkConnectionSuite) TestRunComplainsAboutConnectionErrors(c *gc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(newAgentConf(),
		func(a agent.Agent) (io.Closer, error) {
			return nil, errors.Errorf("hartz-timor swarm detected")
		})
	c.Assert(cmd.Init([]string{"unit-artemis-5"}), jc.ErrorIsNil)
	err := cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, "checking connection for unit-artemis-5: hartz-timor swarm detected")
}

func (s *checkConnectionSuite) TestRunClosesConnection(c *gc.C) {
	cmd := agentcmd.NewCheckConnectionCommand(newAgentConf(),
		func(a agent.Agent) (io.Closer, error) {
			return &mockConnection{}, nil
		})
	c.Assert(cmd.Init([]string{"unit-artemis-5"}), jc.ErrorIsNil)
	err := cmd.Run(nil)
	c.Assert(err, gc.ErrorMatches, "closing connection for unit-artemis-5: seal integrity check failed")
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
