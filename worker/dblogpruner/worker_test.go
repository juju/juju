// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/dblogpruner"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var _ = gc.Suite(&suite{})

type suite struct {
	jujutesting.MgoSuite
	testing.BaseSuite

	state          *state.State
	pruner         worker.Worker
	logsColl       *mgo.Collection
	controllerColl *mgo.Collection
}

func (s *suite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *suite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *suite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)
}

func (s *suite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *suite) setupState(c *gc.C, maxLogAge, maxCollectionMB string) {
	controllerConfig := map[string]interface{}{
		"max-logs-age":  maxLogAge,
		"max-logs-size": maxCollectionMB,
	}

	ctlr := statetesting.InitializeWithArgs(c, statetesting.InitializeArgs{
		Owner:            names.NewLocalUserTag("test-admin"),
		Clock:            testclock.NewClock(testing.NonZeroTime()),
		ControllerConfig: controllerConfig,
	})
	s.AddCleanup(func(*gc.C) { ctlr.Close() })
	s.state = ctlr.SystemState()
	s.logsColl = s.state.MongoSession().DB("logs").C("logs." + s.state.ModelUUID())
}

func (s *suite) startWorker(c *gc.C) {
	pruner, err := dblogpruner.NewWorker(dblogpruner.Config{
		State:         s.state,
		Clock:         clock.WallClock,
		PruneInterval: time.Millisecond,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.pruner = pruner
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, s.pruner) })
}

func (s *suite) TestWorkerHasReport(c *gc.C) {
	s.setupState(c, "24h", "1024M")
	s.startWorker(c)

	type reporter interface {
		Report() map[string]interface{}
	}

	r, ok := s.pruner.(reporter)
	c.Assert(ok, jc.IsTrue)
	report := r.Report()
	// We can't generally check on particular values except that there are keys for
	// prune-age and prune-size.
	_, ok = report["prune-age"]
	c.Assert(ok, jc.IsTrue)
	_, ok = report["prune-size"]
	c.Assert(ok, jc.IsTrue)
}

func (s *suite) TestPrunesOldLogs(c *gc.C) {
	maxLogAge := 24 * time.Hour
	s.setupState(c, "24h", "1000P")
	s.startWorker(c)

	now := time.Now()
	addLogsToPrune := func(count int) {
		// Add messages beyond the prune threshold.
		tPrune := now.Add(-maxLogAge - 1)
		s.addLogs(c, tPrune, "prune", count)
	}
	addLogsToKeep := func(count int) {
		// Add messages within the prune threshold.
		s.addLogs(c, now, "keep", count)
	}
	for i := 0; i < 10; i++ {
		addLogsToKeep(5)
		addLogsToPrune(5)
	}

	// Wait for all logs with the message "prune" to be removed.
	for attempt := testing.LongAttempt.Start(); attempt.Next(); {
		pruneRemaining, err := s.logsColl.Find(bson.M{"x": "prune"}).Count()
		c.Assert(err, jc.ErrorIsNil)
		if pruneRemaining == 0 {
			// All the "keep" messages should still be there.
			keepCount, err := s.logsColl.Find(bson.M{"x": "keep"}).Count()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(keepCount, gc.Equals, 50)
			return
		}
	}
	c.Fatal("pruning didn't happen as expected")
}

type storageEngine struct {
	Name string `bson:"name"`
}
type serverStatus struct {
	StorageEngine storageEngine `bson:"storageEngine"`
}

func (s *suite) TestPrunesLogsBySize(c *gc.C) {
	s.setupState(c, "999h", "2M")
	var res serverStatus
	err := s.Session.Run(bson.M{"serverStatus": 1}, &res)
	c.Assert(err, jc.ErrorIsNil)
	startingLogCount := 25000
	// wiredTiger compresses the size on disk by default, so it takes more
	// effort to get the logs to be pruned.
	if res.StorageEngine.Name == "wiredTiger" {
		startingLogCount *= 2
	}
	s.addLogs(c, time.Now(), "stuff", startingLogCount)

	s.startWorker(c)
	for attempt := testing.LongAttempt.Start(); attempt.Next(); {
		count, err := s.logsColl.Count()
		c.Assert(err, jc.ErrorIsNil)
		// The space used by MongoDB by the collection isn't that
		// predictable, so just treat any pruning due to size as
		// success.
		if count < startingLogCount {
			return
		}
	}
	c.Fatal("pruning didn't happen as expected")
}

func (s *suite) addLogs(c *gc.C, t0 time.Time, text string, count int) {
	dbLogger := state.NewDbLogger(s.state)
	defer dbLogger.Close()

	for offset := 0; offset < count; offset++ {
		t := t0.Add(-time.Duration(offset) * time.Second)
		dbLogger.Log([]state.LogRecord{{
			Time:     t,
			Entity:   names.NewMachineTag("0"),
			Version:  version.Current,
			Module:   "some.module",
			Location: "foo.go:42",
			Level:    loggo.INFO,
			Message:  text,
		}})
	}
}
