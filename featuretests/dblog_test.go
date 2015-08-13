// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"bufio"
	"io/ioutil"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	"github.com/juju/juju/cmd/jujud/util/password"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/peergrouper"
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
	s.PatchValue(&password.EnsureJujudPassword, func() error { return nil })

	// Change the path to "juju-run", so that the
	// tests don't try to write to /usr/local/bin.
	file, _ := ioutil.TempFile("", "juju-run")
	defer file.Close()
	s.PatchValue(&agentcmd.JujuRun, file.Name())
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
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(agentConf, logsCh, nil)
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

// debugLogDbSuite tests that the debuglog API works when logs are
// being read from the database.
type debugLogDbSuite struct {
	agenttesting.AgentSuite
}

var _ = gc.Suite(&debugLogDbSuite{})

func (s *debugLogDbSuite) SetUpSuite(c *gc.C) {
	s.SetInitialFeatureFlags("db-log")

	// Restart mongod with a the replicaset enabled.
	mongod := jujutesting.MgoServer
	mongod.Params = []string{"--replSet", "juju"}
	mongod.Restart()

	// Initiate the replicaset.
	info := mongod.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: mongod.Addr(),
	}
	err := peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	s.AgentSuite.SetUpSuite(c)
}

func (s *debugLogDbSuite) TearDownSuite(c *gc.C) {
	// Restart mongod without the replicaset enabled so as not to
	// affect other test that reply on this mongod instance in this
	// package.
	mongod := jujutesting.MgoServer
	mongod.Params = []string{}
	mongod.Restart()

	s.AgentSuite.TearDownSuite(c)
}

func (s *debugLogDbSuite) TestLogsAPI(c *gc.C) {
	dbLogger := state.NewDbLogger(s.State, names.NewMachineTag("99"))
	defer dbLogger.Close()

	t := time.Date(2015, 6, 23, 13, 8, 49, 0, time.UTC)
	dbLogger.Log(t, "juju.foo", "code.go:42", loggo.INFO, "all is well")
	dbLogger.Log(t.Add(time.Second), "juju.bar", "go.go:99", loggo.ERROR, "no it isn't")

	lines := make(chan string)
	go func(numLines int) {
		client := s.APIState.Client()
		reader, err := client.WatchDebugLog(api.DebugLogParams{})
		c.Assert(err, jc.ErrorIsNil)
		defer reader.Close()

		bufReader := bufio.NewReader(reader)
		for n := 0; n < numLines; n++ {
			line, err := bufReader.ReadString('\n')
			c.Assert(err, jc.ErrorIsNil)
			lines <- line
		}
	}(3)

	assertLine := func(expected string) {
		select {
		case actual := <-lines:
			c.Assert(actual, gc.Equals, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log line")
		}
	}

	// Read the 2 lines that are in the logs collection.
	assertLine("machine-99: 2015-06-23 13:08:49 INFO juju.foo code.go:42 all is well\n")
	assertLine("machine-99: 2015-06-23 13:08:50 ERROR juju.bar go.go:99 no it isn't\n")

	// Now write and observe another log. This should be read from the oplog.
	dbLogger.Log(t.Add(2*time.Second), "ju.jitsu", "no.go:3", loggo.WARNING, "beep beep")
	assertLine("machine-99: 2015-06-23 13:08:51 WARNING ju.jitsu no.go:3 beep beep\n")
}
