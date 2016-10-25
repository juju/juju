// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/logsender"
)

type introspectionSuite struct {
	agenttest.AgentSuite
	logger *logsender.BufferedLogWriter
}

var _ = gc.Suite(&introspectionSuite{})

func (s *introspectionSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	if runtime.GOOS != "linux" {
		c.Skip(fmt.Sprintf("the introspection worker does not support %q", runtime.GOOS))
	}
}

func (s *introspectionSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)

	var err error
	s.logger, err = logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		err := logsender.UninstallBufferedLogWriter()
		c.Assert(err, jc.ErrorIsNil)
	})
}

// startMachineAgent starts a machine agent and returns the path
// of its unix socket.
func (s *introspectionSuite) startMachineAgent(c *gc.C) (*agentcmd.MachineAgent, string) {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: agent.BootstrapNonce,
	})

	s.PrimeAgent(c, m.Tag(), password)
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(m.Tag().String())

	rootDir := c.MkDir()
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		s.logger,
		func(names.Tag) string { return rootDir },
		rootDir,
	)
	a := machineAgentFactory(m.Id())

	// Start the agent.
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	// Wait for the agent to start serving on the introspection socket.
	var conn net.Conn
	for a := testing.LongAttempt.Start(); a.Next(); {
		var err error
		conn, err = net.Dial("unix", "@"+rootDir)
		if err == nil {
			break
		}
	}
	if conn == nil {
		a.Stop()
		c.Fatal("timed out waiting for introspection socket")
	}
	conn.Close()
	return a, "@" + rootDir
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}

func (s *introspectionSuite) TestPrometheusMetrics(c *gc.C) {
	a, socketPath := s.startMachineAgent(c)
	defer a.Stop()
	client := unixSocketHTTPClient(socketPath)

	resp, err := client.Get("http://unix.socket/metrics")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), jc.Contains, "juju_logsender_capacity 1000")

	// NOTE(axw) the "juju_api_*" metrics are not currently
	// registered in the test, because AgentSuite is based
	// on top of JujuConnSuite. JujuConnSuite uses the dummy
	// provider's API server, rather than the one that the
	// agent starts. Because of that, the usual observers
	// do not apply.
}
