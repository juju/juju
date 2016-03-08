// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender_test

import (
	"fmt"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/api"
	apilogsender "github.com/juju/juju/api/logsender"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/logsender"
)

type workerSuite struct {
	jujutesting.JujuConnSuite

	// machineTag holds the tag of a machine created
	// for the test.
	machineTag names.Tag

	// APIState holds an API connection authenticated
	// as the above machine.
	APIState api.Connection
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine for the client to log in as.
	nonce := "some-nonce"
	machine, password := s.Factory.MakeMachineReturningPassword(c,
		&factory.MachineParams{Nonce: nonce})
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = machine.Tag()
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	st, err := api.Open(apiInfo, api.DefaultDialOpts())
	c.Assert(err, gc.IsNil)
	s.APIState = st
	s.machineTag = machine.Tag()
}

func (s *workerSuite) TearDownTest(c *gc.C) {
	s.APIState.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *workerSuite) logSenderAPI() *apilogsender.API {
	return apilogsender.NewAPI(s.APIState)
}

func (s *workerSuite) TestLogSending(c *gc.C) {
	const logCount = 5
	logsCh := make(chan *logsender.LogRecord, logCount)

	// Start the logsender worker.
	worker := logsender.New(logsCh, s.logSenderAPI())
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
			"e": s.State.ModelUUID(),
			"n": s.machineTag.String(),
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
	worker := logsender.New(logsCh, s.logSenderAPI())
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
		"e": s.State.ModelUUID(),
		"n": s.machineTag.String(),
		"m": "juju.worker.logsender",
		"l": "",
		"v": int(loggo.WARNING),
		"x": "42 log messages dropped due to lack of API connectivity",
	})
	c.Assert(docs[2]["x"], gc.Equals, "message1")
}
