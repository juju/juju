// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
)

type LogsSuite struct {
	ConnSuite
	logsColl *mgo.Collection
}

var _ = gc.Suite(&LogsSuite{})

func (s *LogsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	session := s.State.MongoSession()
	s.logsColl = session.DB("logs").C("logs")
}

func (s *LogsSuite) TestIndexesCreated(c *gc.C) {
	// Indexes should be created on the logs collection when state is opened.
	indexes, err := s.logsColl.Indexes()
	c.Assert(err, jc.ErrorIsNil)
	var keys []string
	for _, index := range indexes {
		keys = append(keys, strings.Join(index.Key, "-"))
	}
	c.Assert(keys, jc.SameContents, []string{
		"_id", // default index
		"e-t", // env-uuid and timestamp
		"e-n", // env-uuid and entity
	})
}

func (s *LogsSuite) TestDbLogger(c *gc.C) {
	logger := state.NewDbLogger(s.State, names.NewMachineTag("22"))
	defer logger.Close()
	t0 := time.Now().Truncate(time.Millisecond) // MongoDB only stores timestamps with ms precision.
	logger.Log(t0, "some.where", "foo.go:99", loggo.INFO, "all is well")
	t1 := t0.Add(time.Second)
	logger.Log(t1, "else.where", "bar.go:42", loggo.ERROR, "oh noes")

	var docs []bson.M
	err := s.logsColl.Find(nil).Sort("t").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 2)

	c.Assert(docs[0]["t"], gc.Equals, t0)
	c.Assert(docs[0]["e"], gc.Equals, s.State.EnvironUUID())
	c.Assert(docs[0]["n"], gc.Equals, "machine-22")
	c.Assert(docs[0]["m"], gc.Equals, "some.where")
	c.Assert(docs[0]["l"], gc.Equals, "foo.go:99")
	c.Assert(docs[0]["v"], gc.Equals, int(loggo.INFO))
	c.Assert(docs[0]["x"], gc.Equals, "all is well")

	c.Assert(docs[1]["t"], gc.Equals, t1)
	c.Assert(docs[1]["e"], gc.Equals, s.State.EnvironUUID())
	c.Assert(docs[1]["n"], gc.Equals, "machine-22")
	c.Assert(docs[1]["m"], gc.Equals, "else.where")
	c.Assert(docs[1]["l"], gc.Equals, "bar.go:42")
	c.Assert(docs[1]["v"], gc.Equals, int(loggo.ERROR))
	c.Assert(docs[1]["x"], gc.Equals, "oh noes")
}

func (s *LogsSuite) TestPruneLogsByTime(c *gc.C) {
	dbLogger := state.NewDbLogger(s.State, names.NewMachineTag("22"))
	defer dbLogger.Close()
	log := func(t time.Time, msg string) {
		err := dbLogger.Log(t, "module", "loc", loggo.INFO, msg)
		c.Assert(err, jc.ErrorIsNil)
	}

	now := time.Now()
	maxLogTime := now.Add(-time.Minute)
	log(now, "keep")
	log(maxLogTime.Add(time.Second), "keep")
	log(maxLogTime, "keep")
	log(maxLogTime.Add(-time.Second), "prune")
	log(maxLogTime.Add(-(2 * time.Second)), "prune")

	sizeNoPrune := int(1e10)
	err := state.PruneLogs(s.State, maxLogTime, sizeNoPrune)
	c.Assert(err, jc.ErrorIsNil)

	// After pruning there should just be 3 "keep" messages left.
	var docs []bson.M
	err = s.logsColl.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 3)
	for _, doc := range docs {
		c.Assert(doc["x"], gc.Equals, "keep")
	}
}

func (s *LogsSuite) TestPruneLogsBySize(c *gc.C) {
	// Set up 3 environments and generate different amounts of logs
	// for them.
	now := time.Now().Truncate(time.Millisecond)

	s0 := s.State
	s.generateLogs(c, s0, now, 10)

	s1 := s.factory.MakeEnvironment(c, nil)
	defer s1.Close()
	s.generateLogs(c, s1, now, 6000)

	s2 := s.factory.MakeEnvironment(c, nil)
	defer s2.Close()
	s.generateLogs(c, s2, now, 7000)

	// Prune logs collection back by size.
	tsNoPrune := time.Now().Add(-3 * 24 * time.Hour)
	err := state.PruneLogs(s.State, tsNoPrune, 2500000)
	c.Assert(err, jc.ErrorIsNil)

	// Check logs were pruned as expected.
	c.Assert(s.countLogs(c, s0), gc.Equals, 10) // Not touched
	s.assertLogCountBetween(c, s1, 5100, 5200)  // Should be fairly evenly truncated.
	s.assertLogCountBetween(c, s2, 5100, 5200)

	// Ensure that the latest log records are still there.
	assertLatestTs := func(st *state.State) {
		var doc bson.M
		err := s.logsColl.Find(bson.M{"e": st.EnvironUUID()}).Sort("-t").One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(doc["t"].(time.Time), gc.Equals, now)
	}
	assertLatestTs(s0)
	assertLatestTs(s1)
	assertLatestTs(s2)
}

func (s *LogsSuite) TestPruneLogsWithSmallSizeThreshold(c *gc.C) {
	// Check behaviour with an unlikely and pathological collection
	// size limit. Previous implementations of PruneLogs could error
	// out in this situation.

	now := time.Now()
	s.generateLogs(c, s.State, now, 6000)

	tsNoPrune := now.Add(-3 * 24 * time.Hour)
	tinySize := 100
	err := state.PruneLogs(s.State, tsNoPrune, tinySize)
	c.Assert(err, jc.ErrorIsNil)

	s.assertLogCountBetween(c, s.State, 4900, 5000)
}

func (s *LogsSuite) generateLogs(c *gc.C, st *state.State, now time.Time, count int) {
	dbLogger := state.NewDbLogger(st, names.NewMachineTag("0"))
	defer dbLogger.Close()
	for i := 0; i < count; i++ {
		ts := now.Add(-time.Duration(i) * time.Second)
		err := dbLogger.Log(ts, "module", "loc", loggo.INFO, "message")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *LogsSuite) assertLogCountBetween(c *gc.C, st *state.State, min, max int) {
	count := s.countLogs(c, st)
	c.Assert(count, jc.LessThan, max)
	c.Assert(count, jc.GreaterThan, min)
}

func (s *LogsSuite) countLogs(c *gc.C, st *state.State) int {
	count, err := s.logsColl.Find(bson.M{"e": st.EnvironUUID()}).Count()
	c.Assert(err, jc.ErrorIsNil)
	return count
}
