// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/v2/core/logger"
	coretesting "github.com/juju/juju/v2/testing"
	"github.com/juju/juju/v2/worker/syslogger"
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
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			s.stub.MethodCall(s, "NewLogger", priority, tag)
			return nil, nil
		},
	})
	c.Assert(err, gc.IsNil)
	s.stub.CheckCallNames(c, strings.Split(strings.Repeat("NewLogger,", 7), ",")[:7]...)
	for _, call := range s.stub.Calls() {
		arg := call.Args[0].(syslogger.Priority)
		c.Assert(arg >= syslogger.LOG_CRIT && arg <= syslogger.LOG_DEBUG, gc.Equals, true)
	}
}

func (s *WorkerSuite) TestLog(c *gc.C) {
	now := time.Now()
	buf := new(bytes.Buffer)
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return closer{buf}, nil
		},
	})
	c.Assert(err, gc.IsNil)
	wrk := w.(syslogger.SysLogger)
	err = wrk.Log([]corelogger.LogRecord{{
		Time:      now,
		Entity:    "foo",
		Module:    "bar",
		Message:   "baz",
		ModelUUID: coretesting.ModelTag.Id(),
	}})
	c.Assert(err, gc.IsNil)

	dateTime := now.In(time.UTC).Format("2006-01-02 15:04:05")
	c.Assert(buf.String(), gc.Equals, fmt.Sprintf("%s foo bar.deadbe baz\n", dateTime))
}

func (s *WorkerSuite) TestClosingLogBeforeWriting(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockWriter := syslogger.NewMockWriteCloser(ctrl)
	mockWriter.EXPECT().Close().Times(7)

	now := time.Now()
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return mockWriter, nil
		},
	})
	c.Assert(err, gc.IsNil)

	w.Kill()
	c.Assert(w.Wait(), gc.IsNil)

	wrk := w.(syslogger.SysLogger)
	err = wrk.Log([]corelogger.LogRecord{{
		Time:      now,
		Entity:    "foo",
		Module:    "bar",
		Message:   "baz",
		ModelUUID: coretesting.ModelTag.Id(),
	}})
	c.Assert(err, gc.IsNil)
}

func (s *WorkerSuite) TestClosingLogWhilstWriting(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockWriter := syslogger.NewMockWriteCloser(ctrl)
	mockWriter.EXPECT().Write(gomock.Any()).MinTimes(1)
	mockWriter.EXPECT().Close().Times(7)

	now := time.Now()
	w, err := syslogger.NewWorker(syslogger.WorkerConfig{
		NewLogger: func(priority syslogger.Priority, tag string) (io.WriteCloser, error) {
			return mockWriter, nil
		},
	})
	c.Assert(err, gc.IsNil)

	done := make(chan struct{})
	go func() {
		c.Assert(w.Wait(), gc.IsNil)
		close(done)
	}()
	go func() {
		wrk := w.(syslogger.SysLogger)
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Millisecond):
				err = wrk.Log([]corelogger.LogRecord{{
					Time:      now,
					Entity:    "foo",
					Module:    "bar",
					Message:   "baz",
					ModelUUID: coretesting.ModelTag.Id(),
				}})
				c.Assert(err, gc.IsNil)
			}
		}
	}()
	go func() {
		<-time.After(time.Millisecond * 10)
		w.Kill()
	}()
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatal("failed waiting for test to complete")
	}
}

type closer struct {
	io.Writer
}

func (c closer) Close() error { return nil }
