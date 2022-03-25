// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger_test

import (
	"bytes"
	"fmt"
	"io"
	"log/syslog"
	"strings"
	"time"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/syslogger"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type WorkerSuite struct {
	stub testing.Stub
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.stub.ResetCalls()
}

func (s *WorkerSuite) TestLogCreation(c *gc.C) {
	_, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslog.Priority, tag string) (io.WriteCloser, error) {
			s.stub.MethodCall(s, "NewLogger", priority, tag)
			return nil, nil
		},
	})
	c.Assert(err, gc.IsNil)
	s.stub.CheckCallNames(c, strings.Split(strings.Repeat("NewLogger,", 7), ",")[:7]...)
	for _, call := range s.stub.Calls() {
		arg := call.Args[0].(syslog.Priority)
		c.Assert(arg >= syslog.LOG_CRIT && arg <= syslog.LOG_DEBUG, gc.Equals, true)
	}
}

func (s *WorkerSuite) TestLog(c *gc.C) {
	now := time.Now()
	buf := new(bytes.Buffer)
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslog.Priority, tag string) (io.WriteCloser, error) {
			return closer{buf}, nil
		},
	})
	c.Assert(err, gc.IsNil)
	err = w.Log([]state.LogRecord{{
		Time:    now,
		Entity:  "foo",
		Module:  "bar",
		Message: "baz",
	}})
	c.Assert(err, gc.IsNil)

	dateTime := now.In(time.UTC).Format("2006-01-02 15:04:05")
	c.Assert(buf.String(), gc.Equals, fmt.Sprintf("%s foo bar baz\n", dateTime))
}

type closer struct {
	io.Writer
}

func (c closer) Close() error { return nil }
