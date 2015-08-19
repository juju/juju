// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/logsender"
)

type workerSuite struct {
	jujutesting.JujuConnSuite
	apiInfo *api.Info
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags("db-log")
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine for the client to log in as.
	nonce := "some-nonce"
	machine, password := s.Factory.MakeMachineReturningPassword(c,
		&factory.MachineParams{Nonce: nonce})
	s.apiInfo = s.APIInfo(c)
	s.apiInfo.Tag = machine.Tag()
	s.apiInfo.Password = password
	s.apiInfo.Nonce = nonce
}

func (s *workerSuite) agent() agent.Agent {
	return &mockAgent{apiInfo: s.apiInfo}
}

func (s *workerSuite) TestLockedGate(c *gc.C) {

	// Set a bad password to induce an error if we connect.
	s.apiInfo.Password = "lol-borken"

	// Run a logsender worker.
	logsCh := make(chan *logsender.LogRecord)
	worker := logsender.New(logsCh, lockedGate{}, s.agent())

	// At the end of the test, make sure we never tried to connect.
	defer func() {
		worker.Kill()
		c.Check(worker.Wait(), jc.ErrorIsNil)
	}()

	// Give it a chance to ignore the gate and read the log channel.
	select {
	case <-time.After(testing.ShortWait):
	case logsCh <- &logsender.LogRecord{}:
		c.Fatalf("read log channel without waiting for gate")
	}
}

func (s *workerSuite) TestLogSending(c *gc.C) {
	const logCount = 5
	logsCh := make(chan *logsender.LogRecord, logCount)

	// Start the logsender worker.
	worker := logsender.New(logsCh, gate.AlreadyUnlocked{}, s.agent())
	defer func() {
		worker.Kill()
		c.Check(worker.Wait(), jc.ErrorIsNil)
	}()

	// Send some logs, also building up what should appear in the
	// database.
	var expectedDocs []bson.M
	for i := 0; i < logCount; i++ {
		ts := time.Now().Truncate(time.Millisecond)
		location := fmt.Sprintf("loc%d", i)
		message := fmt.Sprintf("%d", i)

		logsCh <- &logsender.LogRecord{
			Time:     ts,
			Module:   "logsender-test",
			Location: location,
			Level:    loggo.INFO,
			Message:  message,
		}

		expectedDocs = append(expectedDocs, bson.M{
			"t": ts,
			"e": s.State.EnvironUUID(),
			"n": s.apiInfo.Tag.String(),
			"m": "logsender-test",
			"l": location,
			"v": int(loggo.INFO),
			"x": message,
		})
	}

	// Wait for the logs to appear in the database.
	var docs []bson.M
	logsColl := s.State.MongoSession().DB("logs").C("logs")
	for a := testing.LongAttempt.Start(); a.Next(); {
		err := logsColl.Find(bson.M{"m": "logsender-test"}).All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		if len(docs) == logCount {
			break
		}
	}

	// Check that the logs are correct.
	c.Assert(docs, gc.HasLen, logCount)
	for i := 0; i < logCount; i++ {
		doc := docs[i]
		delete(doc, "_id")
		c.Assert(doc, gc.DeepEquals, expectedDocs[i])
	}
}

func (s *workerSuite) TestDroppedLogs(c *gc.C) {
	logsCh := make(logsender.LogRecordCh)

	// Start the logsender worker.
	worker := logsender.New(logsCh, gate.AlreadyUnlocked{}, s.agent())
	defer func() {
		worker.Kill()
		c.Check(worker.Wait(), jc.ErrorIsNil)
	}()

	// Send a log record which indicates some messages after it were
	// dropped.
	ts := time.Now().Truncate(time.Millisecond)
	logsCh <- &logsender.LogRecord{
		Time:         ts,
		Module:       "aaa",
		Location:     "loc",
		Level:        loggo.INFO,
		Message:      "message0",
		DroppedAfter: 42,
	}

	// Send another log record with no drops indicated.
	logsCh <- &logsender.LogRecord{
		Time:     time.Now(),
		Module:   "zzz",
		Location: "loc",
		Level:    loggo.INFO,
		Message:  "message1",
	}

	// Wait for the logs to appear in the database.
	var docs []bson.M
	logsColl := s.State.MongoSession().DB("logs").C("logs")
	for a := testing.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Fatal("timed out waiting for logs")
		}
		err := logsColl.Find(nil).Sort("m").All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		// Expect the 2 messages sent along with a message about
		// dropped messages.
		if len(docs) == 3 {
			break
		}
	}

	// Check that the log records sent are present as well as an additional
	// message in between indicating that some messages were dropped.
	c.Assert(docs[0]["x"], gc.Equals, "message0")
	delete(docs[1], "_id")
	c.Assert(docs[1], gc.DeepEquals, bson.M{
		"t": ts, // Should share timestamp with previous message.
		"e": s.State.EnvironUUID(),
		"n": s.apiInfo.Tag.String(),
		"m": "juju.worker.logsender",
		"l": "",
		"v": int(loggo.WARNING),
		"x": "42 log messages dropped due to lack of API connectivity",
	})
	c.Assert(docs[2]["x"], gc.Equals, "message1")
}

type mockAgent struct {
	agent.Agent
	agent.Config
	apiInfo *api.Info
}

func (a *mockAgent) CurrentConfig() agent.Config {
	return a
}

func (a *mockAgent) APIInfo() *api.Info {
	return a.apiInfo
}

type lockedGate struct{}

func (lockedGate) Unlocked() <-chan struct{} {
	return nil
}
