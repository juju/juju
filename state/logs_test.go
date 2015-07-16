// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"strconv"
	"strings"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
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

	noPruneMB := 100
	err := state.PruneLogs(s.State, maxLogTime, noPruneMB)
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
	startingLogsS0 := 10
	s.generateLogs(c, s0, now, startingLogsS0)

	s1 := s.Factory.MakeEnvironment(c, nil)
	defer s1.Close()
	startingLogsS1 := 10000
	s.generateLogs(c, s1, now, startingLogsS1)

	s2 := s.Factory.MakeEnvironment(c, nil)
	defer s2.Close()
	startingLogsS2 := 12000
	s.generateLogs(c, s2, now, startingLogsS2)

	// Prune logs collection back to 1 MiB.
	tsNoPrune := time.Now().Add(-3 * 24 * time.Hour)
	err := state.PruneLogs(s.State, tsNoPrune, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Logs for first env should not be touched.
	c.Assert(s.countLogs(c, s0), gc.Equals, startingLogsS0)

	// Logs for second env should be pruned.
	c.Assert(s.countLogs(c, s1), jc.LessThan, startingLogsS1)

	// Logs for third env should be pruned to a similar level as
	// second env.
	c.Assert(s.countLogs(c, s2), jc.LessThan, startingLogsS1)

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

func (s *LogsSuite) generateLogs(c *gc.C, st *state.State, endTime time.Time, count int) {
	dbLogger := state.NewDbLogger(st, names.NewMachineTag("0"))
	defer dbLogger.Close()
	for i := 0; i < count; i++ {
		ts := endTime.Add(-time.Duration(i) * time.Second)
		err := dbLogger.Log(ts, "module", "loc", loggo.INFO, "message")
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *LogsSuite) countLogs(c *gc.C, st *state.State) int {
	count, err := s.logsColl.Find(bson.M{"e": st.EnvironUUID()}).Count()
	c.Assert(err, jc.ErrorIsNil)
	return count
}

type LogTailerSuite struct {
	ConnSuite
	logsColl  *mgo.Collection
	oplogColl *mgo.Collection
}

var _ = gc.Suite(&LogTailerSuite{})

func (s *LogTailerSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	session := s.State.MongoSession()
	s.logsColl = session.DB("logs").C("logs")

	// Create a fake oplog collection.
	s.oplogColl = session.DB("logs").C("oplog.fake")
	err := s.oplogColl.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 1024 * 1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { s.oplogColl.DropCollection() })
}

func (s *LogTailerSuite) TestTimeFiltering(c *gc.C) {
	// Add 10 logs that shouldn't be returned.
	threshT := time.Now()
	s.writeLogsT(c,
		threshT.Add(-5*time.Second), threshT.Add(-time.Millisecond), 5,
		logTemplate{Message: "dont want"},
	)

	// Add 5 logs that should be returned.
	want := logTemplate{Message: "want"}
	s.writeLogsT(c, threshT, threshT.Add(5*time.Second), 5, want)
	tailer := state.NewLogTailer(s.State, &state.LogTailerParams{
		StartTime: threshT,
		Oplog:     s.oplogColl,
	})
	defer tailer.Stop()
	s.assertTailer(c, tailer, 5, want)

	// Write more logs. These will be read from the the oplog.
	want2 := logTemplate{Message: "want 2"}
	s.writeLogsT(c, threshT.Add(6*time.Second), threshT.Add(10*time.Second), 5, want2)
	s.assertTailer(c, tailer, 5, want2)

}

func (s *LogTailerSuite) TestOplogTransition(c *gc.C) {
	// Ensure that logs aren't repeated as the log tailer moves from
	// reading from the logs collection to tailing the oplog.
	//
	// All logs are written out with the same timestamp to create a
	// challenging scenario for the tailer.

	for i := 0; i < 5; i++ {
		s.writeLogs(c, 1, logTemplate{Message: strconv.Itoa(i)})
	}

	tailer := state.NewLogTailer(s.State, &state.LogTailerParams{
		Oplog: s.oplogColl,
	})
	defer tailer.Stop()
	for i := 0; i < 5; i++ {
		s.assertTailer(c, tailer, 1, logTemplate{Message: strconv.Itoa(i)})
	}

	// Write more logs. These will be read from the the oplog.
	for i := 5; i < 10; i++ {
		lt := logTemplate{Message: strconv.Itoa(i)}
		s.writeLogs(c, 2, lt)
		s.assertTailer(c, tailer, 2, lt)
	}
}

func (s *LogTailerSuite) TestEnvironmentFiltering(c *gc.C) {
	good := logTemplate{Message: "good"}
	writeLogs := func() {
		s.writeLogs(c, 1, logTemplate{
			EnvUUID: "someuuid0",
			Message: "bad",
		})
		s.writeLogs(c, 1, logTemplate{
			EnvUUID: "someuuid1",
			Message: "bad",
		})
		s.writeLogs(c, 1, good)
	}

	assert := func(tailer state.LogTailer) {
		// Only the entries the s.State's UUID should be reported.
		s.assertTailer(c, tailer, 1, good)
	}

	s.checkLogTailerFiltering(&state.LogTailerParams{}, writeLogs, assert)
}

func (s *LogTailerSuite) TestLevelFiltering(c *gc.C) {
	info := logTemplate{Level: loggo.INFO}
	error := logTemplate{Level: loggo.ERROR}
	writeLogs := func() {
		s.writeLogs(c, 1, logTemplate{Level: loggo.DEBUG})
		s.writeLogs(c, 1, info)
		s.writeLogs(c, 1, error)
	}
	params := &state.LogTailerParams{
		MinLevel: loggo.INFO,
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 1, info)
		s.assertTailer(c, tailer, 1, error)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestInitialLines(c *gc.C) {
	expected := logTemplate{Message: "want"}
	s.writeLogs(c, 3, logTemplate{Message: "dont want"})
	s.writeLogs(c, 5, expected)

	tailer := state.NewLogTailer(s.State, &state.LogTailerParams{
		InitialLines: 5,
	})
	defer tailer.Stop()

	// Should see just the last 5 lines as requested.
	s.assertTailer(c, tailer, 5, expected)
}

func (s *LogTailerSuite) TestInitialLinesWithNotEnoughLines(c *gc.C) {
	expected := logTemplate{Message: "want"}
	s.writeLogs(c, 2, expected)

	tailer := state.NewLogTailer(s.State, &state.LogTailerParams{
		InitialLines: 5,
	})
	defer tailer.Stop()

	// Should see just the 2 lines that existed, even though 5 were
	// asked for.
	s.assertTailer(c, tailer, 2, expected)
}

func (s *LogTailerSuite) TestIncludeEntity(c *gc.C) {
	machine0 := logTemplate{Entity: names.NewMachineTag("0")}
	foo0 := logTemplate{Entity: names.NewUnitTag("foo/0")}
	foo1 := logTemplate{Entity: names.NewUnitTag("foo/1")}
	writeLogs := func() {
		s.writeLogs(c, 3, machine0)
		s.writeLogs(c, 2, foo0)
		s.writeLogs(c, 1, foo1)
		s.writeLogs(c, 3, machine0)
	}
	params := &state.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo-0",
			"unit-foo-1",
		},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 2, foo0)
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeEntityWildcard(c *gc.C) {
	machine0 := logTemplate{Entity: names.NewMachineTag("0")}
	foo0 := logTemplate{Entity: names.NewUnitTag("foo/0")}
	foo1 := logTemplate{Entity: names.NewUnitTag("foo/1")}
	writeLogs := func() {
		s.writeLogs(c, 3, machine0)
		s.writeLogs(c, 2, foo0)
		s.writeLogs(c, 1, foo1)
		s.writeLogs(c, 3, machine0)
	}
	params := &state.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo*",
		},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 2, foo0)
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeEntity(c *gc.C) {
	machine0 := logTemplate{Entity: names.NewMachineTag("0")}
	foo0 := logTemplate{Entity: names.NewUnitTag("foo/0")}
	foo1 := logTemplate{Entity: names.NewUnitTag("foo/1")}
	writeLogs := func() {
		s.writeLogs(c, 3, machine0)
		s.writeLogs(c, 2, foo0)
		s.writeLogs(c, 1, foo1)
		s.writeLogs(c, 3, machine0)
	}
	params := &state.LogTailerParams{
		ExcludeEntity: []string{
			"machine-0",
			"unit-foo-0",
		},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeEntityWildcard(c *gc.C) {
	machine0 := logTemplate{Entity: names.NewMachineTag("0")}
	foo0 := logTemplate{Entity: names.NewUnitTag("foo/0")}
	foo1 := logTemplate{Entity: names.NewUnitTag("foo/1")}
	writeLogs := func() {
		s.writeLogs(c, 3, machine0)
		s.writeLogs(c, 2, foo0)
		s.writeLogs(c, 1, foo1)
		s.writeLogs(c, 3, machine0)
	}
	params := &state.LogTailerParams{
		ExcludeEntity: []string{
			"machine*",
			"unit-*-0",
		},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 1, foo1)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}
func (s *LogTailerSuite) TestIncludeModule(c *gc.C) {
	mod0 := logTemplate{Module: "foo.bar"}
	mod1 := logTemplate{Module: "juju.thing"}
	subMod1 := logTemplate{Module: "juju.thing.hai"}
	mod2 := logTemplate{Module: "elsewhere"}
	writeLogs := func() {
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, mod1)
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, subMod1)
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, mod2)
	}
	params := &state.LogTailerParams{
		IncludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 1, mod1)
		s.assertTailer(c, tailer, 1, subMod1)
		s.assertTailer(c, tailer, 1, mod2)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestExcludeModule(c *gc.C) {
	mod0 := logTemplate{Module: "foo.bar"}
	mod1 := logTemplate{Module: "juju.thing"}
	subMod1 := logTemplate{Module: "juju.thing.hai"}
	mod2 := logTemplate{Module: "elsewhere"}
	writeLogs := func() {
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, mod1)
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, subMod1)
		s.writeLogs(c, 1, mod0)
		s.writeLogs(c, 1, mod2)
	}
	params := &state.LogTailerParams{
		ExcludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer state.LogTailer) {
		s.assertTailer(c, tailer, 2, mod0)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) TestIncludeExcludeModule(c *gc.C) {
	foo := logTemplate{Module: "foo"}
	bar := logTemplate{Module: "bar"}
	barSub := logTemplate{Module: "bar.thing"}
	baz := logTemplate{Module: "baz"}
	qux := logTemplate{Module: "qux"}
	writeLogs := func() {
		s.writeLogs(c, 1, foo)
		s.writeLogs(c, 1, bar)
		s.writeLogs(c, 1, barSub)
		s.writeLogs(c, 1, baz)
		s.writeLogs(c, 1, qux)
	}
	params := &state.LogTailerParams{
		IncludeModule: []string{"foo", "bar", "qux"},
		ExcludeModule: []string{"foo", "bar"},
	}
	assert := func(tailer state.LogTailer) {
		// Except just "qux" because "foo" and "bar" were included and
		// then excluded.
		s.assertTailer(c, tailer, 1, qux)
	}
	s.checkLogTailerFiltering(params, writeLogs, assert)
}

func (s *LogTailerSuite) checkLogTailerFiltering(
	params *state.LogTailerParams,
	writeLogs func(),
	assertTailer func(state.LogTailer),
) {
	// Check the tailer does the right thing when reading from the
	// logs collection.
	writeLogs()
	params.Oplog = s.oplogColl
	tailer := state.NewLogTailer(s.State, params)
	defer tailer.Stop()
	assertTailer(tailer)

	// Now write out logs and check the tailer again. These will be
	// read from the oplog.
	writeLogs()
	assertTailer(tailer)
}

type logTemplate struct {
	EnvUUID  string
	Entity   names.Tag
	Module   string
	Location string
	Level    loggo.Level
	Message  string
}

// writeLogs creates count log messages at the current time using
// the supplied template. As well as writing to the logs collection,
// entries are also made into the fake oplog collection.
func (s *LogTailerSuite) writeLogs(c *gc.C, count int, lt logTemplate) {
	t := time.Now()
	s.writeLogsT(c, t, t, count, lt)
}

// writeLogsT creates count log messages between startTime and
// endTime using the supplied template. As well as writing to the logs
// collection, entries are also made into the fake oplog collection.
func (s *LogTailerSuite) writeLogsT(c *gc.C, startTime, endTime time.Time, count int, lt logTemplate) {
	s.normaliseLogTemplate(&lt)

	interval := endTime.Sub(startTime) / time.Duration(count)
	t := startTime
	for i := 0; i < count; i++ {
		err := state.WriteLogWithOplog(
			s.oplogColl,
			lt.EnvUUID,
			lt.Entity,
			t,
			lt.Module,
			lt.Location,
			lt.Level,
			lt.Message,
		)
		c.Assert(err, jc.ErrorIsNil)
		t = t.Add(interval)
	}
}

func (s *LogTailerSuite) normaliseLogTemplate(lt *logTemplate) {
	if lt.EnvUUID == "" {
		lt.EnvUUID = s.State.EnvironUUID()
	}
	if lt.Entity == nil {
		lt.Entity = names.NewMachineTag("0")
	}
	if lt.Module == "" {
		lt.Module = "module"
	}
	if lt.Location == "" {
		lt.Location = "loc"
	}
	if lt.Level == loggo.UNSPECIFIED {
		lt.Level = loggo.INFO
	}
	if lt.Message == "" {
		lt.Message = "message"
	}
}

func (s *LogTailerSuite) assertTailer(c *gc.C, tailer state.LogTailer, expectedCount int, lt logTemplate) {
	s.normaliseLogTemplate(&lt)

	timeout := time.After(coretesting.LongWait)
	count := 0
	for {
		select {
		case log, ok := <-tailer.Logs():
			if !ok {
				c.Fatalf("tailer died unexpectedly: %v", tailer.Err())
			}
			c.Assert(log.Entity, gc.Equals, lt.Entity.String())
			c.Assert(log.Module, gc.Equals, lt.Module)
			c.Assert(log.Location, gc.Equals, lt.Location)
			c.Assert(log.Level, gc.Equals, lt.Level)
			c.Assert(log.Message, gc.Equals, lt.Message)
			count++
			if count == expectedCount {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for logs (received %d)", count)
		}
	}
}
