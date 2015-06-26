// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dblogpruner"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

var _ = gc.Suite(&suite{})

type suite struct {
	statetesting.StateSuite
	pruner   worker.Worker
	logsColl *mgo.Collection
}

func (s *suite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.logsColl = s.State.MongoSession().DB("logs").C("logs")
}

func (s *suite) StartWorker(c *gc.C, maxLogAge time.Duration, maxCollectionMB int) {
	params := &dblogpruner.LogPruneParams{
		MaxLogAge:       maxLogAge,
		MaxCollectionMB: maxCollectionMB,
		PruneInterval:   time.Millisecond, // Speed up pruning interval for testing
	}
	s.pruner = dblogpruner.New(s.State, params)
	s.AddCleanup(func(*gc.C) {
		s.pruner.Kill()
		c.Assert(s.pruner.Wait(), jc.ErrorIsNil)
	})
}

func (s *suite) TestPrunesOldLogs(c *gc.C) {
	maxLogAge := 24 * time.Hour
	noPruneMB := int(1e9)
	s.StartWorker(c, maxLogAge, noPruneMB)

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

func (s *suite) TestPrunesLogsBySize(c *gc.C) {
	startingLogCount := 25000
	s.addLogs(c, time.Now(), "stuff", startingLogCount)

	noPruneAge := 999 * time.Hour
	s.StartWorker(c, noPruneAge, 2)

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
	dbLogger := state.NewDbLogger(s.State, names.NewMachineTag("0"))
	defer dbLogger.Close()

	for offset := 0; offset < count; offset++ {
		t := t0.Add(-time.Duration(offset) * time.Second)
		dbLogger.Log(t, "some.module", "foo.go:42", loggo.INFO, text)
	}
}
