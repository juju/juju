// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"
	"net"
	"runtime"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/introspect"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
)

type introspectionSuite struct {
	agenttest.AgentSuite
	logger *logsender.BufferedLogWriter
}

func (s *introspectionSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip(fmt.Sprintf("the introspection worker does not support %q", runtime.GOOS))
	}
	s.AgentSuite.SetUpSuite(c)
	agenttest.InstallFakeEnsureMongo(s)
	s.PatchValue(&agentcmd.ProductionMongoWriteConcern, false)
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

// startMachineAgent starts a controller machine agent and returns the path
// of its unix socket.
func (s *introspectionSuite) startMachineAgent(c *gc.C) (*agentcmd.MachineAgent, string) {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: agent.BootstrapNonce,
	})

	err := m.SetMongoPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	return s.startAgent(c, m.Tag(), password, vers, false)
}

// startMachineAgent starts a controller agent and returns the path
// of its unix socket.
func (s *introspectionSuite) startControllerAgent(c *gc.C) (*agentcmd.MachineAgent, string) {
	// Create a controller node and an agent for it.
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	err = node.SetMongoPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: "kuebernetes",
	}
	return s.startAgent(c, node.Tag(), password, vers, true)
}

func (s *introspectionSuite) startAgent(
	c *gc.C, tag names.Tag, password string, vers version.Binary, isCaas bool,
) (*agentcmd.MachineAgent, string) {
	s.PrimeStateAgentVersion(c, tag, password, vers)
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	err := agentConf.ReadConfig(tag.String())
	c.Assert(err, jc.ErrorIsNil)

	rootDir := c.MkDir()
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		s.logger,
		func(names.Tag) string { return rootDir },
		noPreUpgradeSteps,
		rootDir,
	)
	a, err := machineAgentFactory(tag, isCaas)
	c.Assert(err, jc.ErrorIsNil)

	// Start the agent.
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), jc.ErrorIsNil) }()

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
	return a, rootDir
}

func (s *introspectionSuite) TestPrometheusMetricsControllerAgent(c *gc.C) {
	a, socketPath := s.startControllerAgent(c)
	defer a.Stop()
	s.assertPrometheusMetrics(c, socketPath)
}

func (s *introspectionSuite) TestPrometheusMetricsMachineAgent(c *gc.C) {
	a, socketPath := s.startMachineAgent(c)
	defer a.Stop()
	s.assertPrometheusMetrics(c, socketPath,
		`juju_cache_machines{agent_status="started",instance_status="",life="alive"} 1`,
	)
}

func (s *introspectionSuite) assertPrometheusMetrics(c *gc.C, socketPath string, extra ...string) {
	expected := []string{
		"juju_logsender_capacity 1000",
		"juju_apiserver_connections",
		"juju_mgo_txn_ops_total",
	}
	expected = append(expected, extra...)

	check := func(last bool) bool {
		cmd := introspect.IntrospectCommand{
			IntrospectionSocketName: func(names.Tag) string {
				return socketPath
			},
		}
		ctx, err := cmdtesting.RunCommand(c, &cmd, "--data-dir="+s.DataDir(), "metrics")
		c.Assert(err, jc.ErrorIsNil)
		stdout := cmdtesting.Stdout(ctx)

		for _, expect := range expected {
			if last {
				c.Assert(stdout, jc.Contains, expect)
			} else if !strings.Contains(stdout, expect) {
				return false
			}
		}
		return true
	}

	// Check for metrics in a loop, because the workers might
	// not all have started up initially.
	for a := testing.LongAttempt.Start(); a.Next(); {
		if check(!a.HasNext()) {
			return
		}
	}
	c.Fatal("timed out waiting for metrics")
}
