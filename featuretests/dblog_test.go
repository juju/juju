// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/controller"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/common"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/logsender"
)

// dblogSuite tests that logs flow correctly from the machine and unit
// agents over the API into MongoDB. These are very much integration
// tests with more detailed testing of the individual components
// being done in unit tests.
type dblogSuite struct {
	agenttest.AgentSuite
}

func (s *dblogSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)
}

func (s *dblogSuite) TestControllerAgentLogsGoToDBCAAS(c *gc.C) {
	// Set up a CAAS model to replace the IAAS one.
	st := s.Factory.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	s.State = st
	s.Factory = factory.NewFactory(st, s.StatePool)
	node, err := s.State.AddControllerNode()
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = node.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure controller config matches agent config so the agent worker
	// does not exist with ErrRestartAgent.
	err = s.State.UpdateControllerConfig(map[string]interface{}{
		controller.MongoMemoryProfile: controller.MongoProfLow}, nil)
	c.Assert(err, jc.ErrorIsNil)
	vers := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: "kuebernetes",
	}
	s.PrimeAgentVersion(c, node.Tag(), password, vers)
	s.assertAgentLogsGoToDB(c, node.Tag(), true)
}

func (s *dblogSuite) TestMachineAgentLogsGoToDBIAAS(c *gc.C) {
	// Create a machine and an agent for it.
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce:  agent.BootstrapNonce,
		Series: "quantal",
	})

	s.PrimeAgent(c, m.Tag(), password)
	s.assertAgentLogsGoToDB(c, m.Tag(), false)
}

func (s *dblogSuite) assertAgentLogsGoToDB(c *gc.C, tag names.Tag, isCaas bool) {
	agentConf := agentcmd.NewAgentConf(s.DataDir())
	err := agentConf.ReadConfig(tag.String())
	c.Assert(err, jc.ErrorIsNil)
	logger, err := logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	machineAgentFactory := agentcmd.MachineAgentFactoryFn(
		agentConf,
		logger,
		agentcmd.DefaultIntrospectionSocketName,
		noPreUpgradeSteps,
		c.MkDir(),
	)
	a, err := machineAgentFactory(tag, isCaas)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure there's no logs to begin with.
	c.Assert(s.getLogCount(c, tag), gc.Equals, 0)

	// Start the agent.
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), jc.ErrorIsNil) }()
	defer a.Stop()

	foundLogs := s.waitForLogs(c, tag)
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) TestUnitAgentLogsGoToDB(c *gc.C) {
	// Create a unit and an agent for it.
	u, password := s.Factory.MakeUnitReturningPassword(c, nil)
	s.PrimeAgent(c, u.Tag(), password)
	logger, err := logsender.InstallBufferedLogWriter(1000)
	c.Assert(err, jc.ErrorIsNil)
	a, err := agentcmd.NewUnitAgent(nil, logger)
	c.Assert(err, jc.ErrorIsNil)
	s.InitAgent(c, a, "--unit-name", u.Name(), "--log-to-stderr=true")

	// Ensure there's no logs to begin with.
	c.Assert(s.getLogCount(c, u.Tag()), gc.Equals, 0)

	// Start the agent.
	ctx := cmdtesting.Context(c)
	go func() { c.Assert(a.Run(ctx), jc.ErrorIsNil) }()
	defer a.Stop()

	foundLogs := s.waitForLogs(c, u.Tag())
	c.Assert(foundLogs, jc.IsTrue)
}

func (s *dblogSuite) getLogCount(c *gc.C, entity names.Tag) int {
	// TODO(mjs) - replace this with State's functionality for reading
	// logs from the DB, once it gets this. This will happen before
	// the DB logging feature branch is merged.
	logs := s.Session.DB("logs").C("logs." + s.State.ModelUUID())
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
// NOTE: the actual tests had to be split as the resetting causes
// mongo on bionic to have issues, see note below.
type debugLogDbSuite struct {
	agenttest.AgentSuite
	origReplicaSet bool
}

func (s *debugLogDbSuite) SetUpSuite(c *gc.C) {
	// Ensure mongod has replicaset enabled.
	s.origReplicaSet = jujutesting.MgoServer.EnableReplicaSet
	if !s.origReplicaSet {
		jujutesting.MgoServer.EnableReplicaSet = true
		jujutesting.MgoServer.Restart()
	}
	s.AgentSuite.SetUpSuite(c)
}

func (s *debugLogDbSuite) TearDownSuite(c *gc.C) {
	// Restart mongod without the replicaset enabled so as not to
	// affect other tests that rely on this mongod instance in this
	// package.
	if !s.origReplicaSet {
		jujutesting.MgoServer.EnableReplicaSet = false
		jujutesting.MgoServer.Restart()
	}
	s.AgentSuite.TearDownSuite(c)
}

// NOTE: this is terrible, however due to a bug in mongod on bionic
// when resetting a mongo service with repl set on, we hit an inveriant bug
// which causes the second test to fail always.

// NOTE: do not merge with debugLogDbSuite2
type debugLogDbSuite1 struct {
	debugLogDbSuite
}

func (s *debugLogDbSuite1) TestLogsAPI(c *gc.C) {
	dbLogger := state.NewDbLogger(s.State)
	defer dbLogger.Close()

	t := time.Date(2015, 6, 23, 13, 8, 49, 0, time.UTC)
	err := dbLogger.Log([]state.LogRecord{{
		Time:     t,
		Entity:   "not-a-tag",
		Version:  jujuversion.Current,
		Module:   "juju.foo",
		Location: "code.go:42",
		Level:    loggo.INFO,
		Message:  "all is well",
	}, {
		Time:     t.Add(time.Second),
		Entity:   "not-a-tag",
		Version:  jujuversion.Current,
		Module:   "juju.bar",
		Location: "go.go:99",
		Level:    loggo.ERROR,
		Message:  "no it isn't",
	}})
	c.Assert(err, jc.ErrorIsNil)

	messages := make(chan common.LogMessage)
	go func(numMessages int) {
		client := s.APIState.Client()
		logMessages, err := client.WatchDebugLog(common.DebugLogParams{})
		c.Assert(err, jc.ErrorIsNil)

		for n := 0; n < numMessages; n++ {
			messages <- <-logMessages
		}
	}(3)

	assertMessage := func(expected common.LogMessage) {
		select {
		case actual := <-messages:
			c.Assert(actual, jc.DeepEquals, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log line")
		}
	}

	// Read the 2 lines that are in the logs collection.
	assertMessage(common.LogMessage{
		Entity:    "not-a-tag",
		Timestamp: t,
		Severity:  "INFO",
		Module:    "juju.foo",
		Location:  "code.go:42",
		Message:   "all is well",
	})
	assertMessage(common.LogMessage{
		Entity:    "not-a-tag",
		Timestamp: t.Add(time.Second),
		Severity:  "ERROR",
		Module:    "juju.bar",
		Location:  "go.go:99",
		Message:   "no it isn't",
	})

	// Now write and observe another log. This should be read from the oplog.
	err = dbLogger.Log([]state.LogRecord{{
		Time:     t.Add(2 * time.Second),
		Entity:   "not-a-tag",
		Version:  jujuversion.Current,
		Module:   "ju.jitsu",
		Location: "no.go:3",
		Level:    loggo.WARNING,
		Message:  "beep beep",
	}})
	assertMessage(common.LogMessage{
		Entity:    "not-a-tag",
		Timestamp: t.Add(2 * time.Second),
		Severity:  "WARNING",
		Module:    "ju.jitsu",
		Location:  "no.go:3",
		Message:   "beep beep",
	})
	c.Assert(err, jc.ErrorIsNil)
}

// NOTE: do not merge with debugLogDbSuite1
type debugLogDbSuite2 struct {
	debugLogDbSuite
}

func (s *debugLogDbSuite2) TestLogsUsesStartTime(c *gc.C) {
	dbLogger := state.NewDbLogger(s.State)
	defer dbLogger.Close()

	entity := "not-a-tag"

	vers := jujuversion.Current
	t1 := time.Date(2015, 6, 23, 13, 8, 49, 100, time.UTC)
	// Check that start time has subsecond resolution.
	t2 := time.Date(2015, 6, 23, 13, 8, 51, 50, time.UTC)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(4 * time.Second)
	err := dbLogger.Log([]state.LogRecord{{
		Time:     t1,
		Entity:   entity,
		Version:  vers,
		Module:   "juju.foo",
		Location: "code.go:42",
		Level:    loggo.INFO,
		Message:  "spinto band",
	}, {
		Time:     t2,
		Entity:   entity,
		Version:  vers,
		Module:   "juju.quux",
		Location: "ok.go:101",
		Level:    loggo.INFO,
		Message:  "king gizzard and the lizard wizard",
	}, {
		Time:     t3,
		Entity:   entity,
		Version:  vers,
		Module:   "juju.bar",
		Location: "go.go:99",
		Level:    loggo.ERROR,
		Message:  "born ruffians",
	}, {
		Time:     t4,
		Entity:   entity,
		Version:  vers,
		Module:   "juju.baz",
		Location: "go.go.go:23",
		Level:    loggo.WARNING,
		Message:  "cold war kids",
	}})
	c.Assert(err, jc.ErrorIsNil)

	client := s.APIState.Client()
	logMessages, err := client.WatchDebugLog(common.DebugLogParams{
		StartTime: t3,
	})
	c.Assert(err, jc.ErrorIsNil)

	assertMessage := func(expected common.LogMessage) {
		select {
		case actual := <-logMessages:
			c.Assert(actual, jc.DeepEquals, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for log line")
		}
	}
	assertMessage(common.LogMessage{
		Entity:    "not-a-tag",
		Timestamp: t3,
		Severity:  "ERROR",
		Module:    "juju.bar",
		Location:  "go.go:99",
		Message:   "born ruffians",
	})
	assertMessage(common.LogMessage{
		Entity:    "not-a-tag",
		Timestamp: t4,
		Severity:  "WARNING",
		Module:    "juju.baz",
		Location:  "go.go.go:23",
		Message:   "cold war kids",
	})
}
