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

	"github.com/juju/juju/api"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/worker/logsender"
)

type workerSuite struct {
	jujutesting.JujuConnSuite
	apiInfo *api.Info
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.DbLog)
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

func (s *workerSuite) TestLogSending(c *gc.C) {
	const logCount = 5
	logsCh := make(chan *logsender.LogRecord, logCount)

	// Start the logsender worker.
	worker := logsender.New(logsCh, s.apiInfo)
	defer func() {
		worker.Kill()
		c.Check(worker.Wait(), jc.ErrorIsNil)
	}()

	// Send somes logs, also building up what should appear in the
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
