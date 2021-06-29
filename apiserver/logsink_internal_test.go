// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type loggingStrategySuite struct {
	logs loggo.TestWriter
}

var _ = gc.Suite(&loggingStrategySuite{})

func (s *loggingStrategySuite) TestLoggingOfDBInsertFailures(c *gc.C) {
	var logBuf bytes.Buffer
	strategy := &agentLoggingStrategy{
		dblogger:   failingRecordLogger{},
		fileLogger: &logBuf,
	}

	err := strategy.WriteLog(params.LogRecord{
		Time:    time.Now(),
		Level:   "WARN",
		Message: "running low on resources",
	})

	// The captured DB error should be surfaced from WriteLog
	c.Assert(err, gc.ErrorMatches, ".*spawn more overlords")

	// Ensure that the DB error was also written to the sink
	c.Assert(logBuf.String(), gc.Matches, "(?m).*spawn more overlords.*")
}

type failingRecordLogger struct{}

func (failingRecordLogger) Log([]state.LogRecord) error {
	return errors.New("spawn more overlords")
}
