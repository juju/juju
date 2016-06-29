// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/logfwd"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/logforwarder"
	"github.com/juju/juju/worker/workertest"
)

type LogForwarderSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	stream *stubStream
	sender *stubSender
	rec    logfwd.Record
}

var _ = gc.Suite(&LogForwarderSuite{})

func (s *LogForwarderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.stream = newStubStream(s.stub)
	s.sender = newStubSender(s.stub)
	s.rec = logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: "feebdaed-2f18-4fd2-967d-db9663db7bea",
			ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
			Hostname:       "machine-99.deadbeef-2f18-4fd2-967d-db9663db7bea",
			Type:           logfwd.OriginTypeMachine,
			Name:           "99",
			Software: logfwd.Software{
				PrivateEnterpriseNumber: 28978,
				Name:    "jujud-machine-agent",
				Version: version.Current,
			},
		},
		ID:        10,
		Timestamp: time.Now(),
		Level:     loggo.INFO,
		Location: logfwd.SourceLocation{
			Module:   "api.logstream.test",
			Filename: "test.go",
			Line:     42,
		},
		Message: "test message",
	}
}

func (s *LogForwarderSuite) checkNext(c *gc.C, rec logfwd.Record) {
	s.stream.waitBeforeNext(c)
	s.stream.waitAfterNext(c)
	s.sender.waitAfterSend(c)
	s.stub.CheckCallNames(c, "Next", "Send")
	s.stub.CheckCall(c, 1, "Send", rec)
	s.stub.ResetCalls()
}

func (s *LogForwarderSuite) checkClose(c *gc.C, lf worker.Worker, expected error) {
	go func() {
		s.sender.waitBeforeClose(c)
	}()
	var err error
	if expected == nil {
		workertest.CleanKill(c, lf)
	} else {
		err = workertest.CheckKill(c, lf)
	}
	c.Check(errors.Cause(err), gc.Equals, expected)
	s.stub.CheckCallNames(c, "Close")
}

func (s *LogForwarderSuite) TestOne(c *gc.C) {
	s.stream.setRecords(c, []logfwd.Record{
		s.rec,
	})

	lf, err := logforwarder.NewLogForwarder(s.stream, s.sender)
	c.Assert(err, jc.ErrorIsNil)
	defer s.checkClose(c, lf, nil)

	s.checkNext(c, s.rec)
}

func (s *LogForwarderSuite) TestNoStream(c *gc.C) {
	lf, err := logforwarder.NewLogForwarder(nil, s.sender)
	c.Assert(err, jc.ErrorIsNil)

	s.checkClose(c, lf, nil)
}

func (s *LogForwarderSuite) TestStreamError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, failure)
	s.stream.setRecords(c, []logfwd.Record{
		s.rec,
	})
	lf, err := logforwarder.NewLogForwarder(s.stream, s.sender)
	c.Assert(err, jc.ErrorIsNil)

	s.checkNext(c, s.rec)
	s.stream.waitBeforeNext(c)
	s.stream.waitAfterNext(c)
	s.stub.CheckCallNames(c, "Next")
	s.stub.ResetCalls()
	s.checkClose(c, lf, failure)
}

func (s *LogForwarderSuite) TestSenderError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, nil, nil, failure)
	rec2 := s.rec
	rec2.ID = 11
	s.stream.setRecords(c, []logfwd.Record{
		s.rec,
		rec2,
	})
	lf, err := logforwarder.NewLogForwarder(s.stream, s.sender)
	c.Assert(err, jc.ErrorIsNil)

	s.checkNext(c, s.rec)
	s.checkNext(c, rec2)
	s.checkClose(c, lf, failure)
}

type stubStream struct {
	stub *testing.Stub

	waitCh     chan struct{}
	ReturnNext <-chan logfwd.Record
}

func newStubStream(stub *testing.Stub) *stubStream {
	return &stubStream{
		stub:   stub,
		waitCh: make(chan struct{}),
	}
}

func (s *stubStream) setRecords(c *gc.C, recs []logfwd.Record) {
	recCh := make(chan logfwd.Record)
	go func() {
		for _, rec := range recs {
			select {
			case recCh <- rec:
			case <-time.After(coretesting.LongWait):
				c.Error("timed out waiting for records on the channel")
			}

		}
	}()
	s.ReturnNext = recCh
}

func (s *stubStream) waitBeforeNext(c *gc.C) {
	select {
	case <-s.waitCh:
	case <-time.After(coretesting.LongWait):
		c.Error("timed out waiting")
	}
}

func (s *stubStream) waitAfterNext(c *gc.C) {
	select {
	case <-s.waitCh:
	case <-time.After(coretesting.LongWait):
		c.Error("timed out waiting")
	}
}

func (s *stubStream) Next() (logfwd.Record, error) {
	s.waitCh <- struct{}{}
	s.stub.AddCall("Next")
	s.waitCh <- struct{}{}
	if err := s.stub.NextErr(); err != nil {
		return logfwd.Record{}, errors.Trace(err)
	}

	rec := <-s.ReturnNext
	return rec, nil
}

type stubSender struct {
	stub *testing.Stub

	waitSendCh  chan struct{}
	waitCloseCh chan struct{}
}

func newStubSender(stub *testing.Stub) *stubSender {
	return &stubSender{
		stub:        stub,
		waitSendCh:  make(chan struct{}),
		waitCloseCh: make(chan struct{}),
	}
}

func (s *stubSender) waitAfterSend(c *gc.C) {
	select {
	case <-s.waitSendCh:
	case <-time.After(coretesting.LongWait):
		c.Error("timed out waiting")
	}
}

func (s *stubSender) waitBeforeClose(c *gc.C) {
	select {
	case <-s.waitCloseCh:
	case <-time.After(coretesting.LongWait):
		c.Error("timed out waiting")
	}
}

func (s *stubSender) Send(rec logfwd.Record) error {
	s.stub.AddCall("Send", rec)
	s.waitSendCh <- struct{}{}
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubSender) Close() error {
	s.waitCloseCh <- struct{}{}
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
