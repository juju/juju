// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io/ioutil"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/logsender"
)

// dblogSuite tests that logs flow correctly from the machine and unit
// agents over the API into MongoDB. These are very much integration
// tests with more detailed testing of the individual components
// being done in unit tests.
type dblogSuite struct {
	agenttesting.AgentSuite
}

func (s *dblogSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags("db-log")
	s.AgentSuite.SetUpTest(c)

	// Change the path to "juju-run", so that the
	// tests don't try to write to /usr/local/bin.
	file, _ := ioutil.TempFile("", "juju-run")
	defer file.Close()
	s.PatchValue(&agentcmd.JujuRun, file.Name())

	// If we don't have a lease manager running somewhere, the
	// leadership API calls made by the unit agent hang.
	leaseWorker := worker.NewSimpleWorker(lease.WorkerLoop(s.State))
	s.AddCleanup(func(*gc.C) { worker.Stop(leaseWorker) })
}

func (s *dblogSuite) TestMachineAgentLogsGoToDB(c *gc.C) {
	s.SetFeatureFlags("db-log")
	foundLogs := s.runMachineAgentTest(c)
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) TestMachineAgentLogsGoToDBWithJES(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	foundLogs := s.runMachineAgentTest(c)
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) TestMachineAgentWithoutFeatureFlag(c *gc.C) {
	s.SetFeatureFlags()
	foundLogs := s.runMachineAgentTest(c)
	c.Assert(foundLogs, jc.IsFalse)
}

func (s *dblogSuite) TestUnitAgentLogsGoToDB(c *gc.C) {
	s.SetFeatureFlags("db-log")
	foundLogs := s.runUnitAgentTest(c)
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) TestUnitAgentLogsGoToDBWithJES(c *gc.C) {
	s.SetFeatureFlags(feature.JES)
	foundLogs := s.runUnitAgentTest(c)
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) TestUnitAgentWithoutFeatureFlag(c *gc.C) {
	s.SetFeatureFlags()
	foundLogs := s.runUnitAgentTest(c)
	c.Assert(foundLogs, jc.IsFalse)
}

func (s *dblogSuite) runMachineAgentTest(c *gc.C) bool {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: agent.BootstrapNonce,
	})

	s.PrimeAgent(c, m.Tag(), password, version.Current)
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	agentConf.ReadConfig(m.Tag().String())
	logsCh, err := logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, agentConf, logsCh)
	a := machineAgentFactory(m.Id())

	// Ensure there's no logs to begin with.
	c.Assert(s.getLogCount(c, m.Tag()), gc.Equals, 0)

	// Start the agent.
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer a.Stop()

	return s.waitForLogs(c, m.Tag())
}

func (s *dblogSuite) runUnitAgentTest(c *gc.C) bool {
	// Create a unit and an agent for it.
	u, password := s.Factory.MakeUnitReturningPassword(c, nil)
	s.PrimeAgent(c, u.Tag(), password, version.Current)
	logsCh, err := logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	a := agentcmd.NewUnitAgent(nil, logsCh)
	s.InitAgent(c, a, "--unit-name", u.Name(), "--log-to-stderr=true")

	// Ensure there's no logs to begin with.
	c.Assert(s.getLogCount(c, u.Tag()), gc.Equals, 0)

	// Start the agent.
	go func() { c.Assert(a.Run(nil), jc.ErrorIsNil) }()
	defer a.Stop()

	return s.waitForLogs(c, u.Tag())
}

func (s *dblogSuite) getLogCount(c *gc.C, entity names.Tag) int {
	// TODO(mjs) - replace this with State's functionality for reading
	// logs from the DB, once it gets this. This will happen before
	// the DB logging feature branch is merged.
	logs := s.Session.DB("logs").C("logs")
	count, err := logs.Find(bson.M{"n": entity.String()}).Count()
	c.Assert(err, jc.ErrorIsNil)
	return count
}

func (s *dblogSuite) waitForLogs(c *gc.C, entityTag names.Tag) bool {
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if s.getLogCount(c, entityTag) > 0 {
			return true
		}
	}
	return false
}
