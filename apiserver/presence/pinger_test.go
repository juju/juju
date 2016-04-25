// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/presence"
	coretesting "github.com/juju/juju/testing"
)

type WorkerSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	pinger *stubPinger
	clock  *stubClock
	cfg    presence.Config
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.pinger = &stubPinger{Stub: s.stub}
	s.clock = &stubClock{StubClock: coretesting.NewStubClock(s.stub)}
	s.cfg = presence.Config{
		Identity:   names.NewMachineTag("1"),
		Start:      s.start,
		Clock:      s.clock,
		RetryDelay: time.Nanosecond,
	}
}

func (s *WorkerSuite) start() (presence.Pinger, error) {
	s.stub.AddCall("start")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.pinger, nil
}

func (s *WorkerSuite) TestConfigValidateOkay(c *gc.C) {
	cfg := presence.Config{
		Identity:   names.NewMachineTag("1"),
		Start:      func() (presence.Pinger, error) { return nil, nil },
		Clock:      struct{ clock.Clock }{},
		RetryDelay: time.Second,
	}

	err := cfg.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestConfigValidateNothingUsed(c *gc.C) {
	cfg := presence.Config{
		Identity:   names.NewMachineTag("1"),
		Start:      s.start,
		Clock:      coretesting.NewStubClock(s.stub),
		RetryDelay: time.Second,
	}

	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
}

func (s *WorkerSuite) TestConfigValidateZeroValue(c *gc.C) {
	var cfg presence.Config

	err := cfg.Validate()

	c.Check(err, gc.NotNil)
}

func (s *WorkerSuite) TestConfigValidateMissingIdentity(c *gc.C) {
	cfg := presence.Config{
		Start:      func() (presence.Pinger, error) { return nil, nil },
		Clock:      struct{ clock.Clock }{},
		RetryDelay: time.Second,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*nil Identity.*`)
}

func (s *WorkerSuite) TestConfigValidateMissingStart(c *gc.C) {
	cfg := presence.Config{
		Identity:   names.NewMachineTag("1"),
		Clock:      struct{ clock.Clock }{},
		RetryDelay: time.Second,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*nil Start.*`)
}

func (s *WorkerSuite) TestConfigValidateMissingClock(c *gc.C) {
	cfg := presence.Config{
		Identity:   names.NewMachineTag("1"),
		Start:      func() (presence.Pinger, error) { return nil, nil },
		RetryDelay: time.Second,
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*nil Clock.*`)
}

func (s *WorkerSuite) TestConfigValidateMissingRetryDelay(c *gc.C) {
	cfg := presence.Config{
		Identity: names.NewMachineTag("1"),
		Start:    func() (presence.Pinger, error) { return nil, nil },
		Clock:    struct{ clock.Clock }{},
	}

	err := cfg.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `.*non-positive RetryDelay.*`)
}

func (s *WorkerSuite) TestNewRunOnceBeforeLoop(c *gc.C) {
	waitChan := make(chan struct{})
	s.clock.notify = waitChan

	w, err := presence.New(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()
	<-waitChan

	s.stub.CheckCallNames(c,
		"start",
		"Wait",
		"After",
	)
}

func (s *WorkerSuite) TestNewFailStart(c *gc.C) {
	waitChan := make(chan struct{})
	s.clock.notify = waitChan
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	w, err := presence.New(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()
	<-waitChan

	s.stub.CheckCallNames(c,
		"start",
		"After", // continued on
	)
}

func (s *WorkerSuite) TestNewFailWait(c *gc.C) {
	waitChan := make(chan struct{})
	s.clock.notify = waitChan
	failure := errors.New("<failure>")
	s.stub.SetErrors(nil, failure)

	w, err := presence.New(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()
	<-waitChan

	s.stub.CheckCallNames(c,
		"start",
		"Wait",
		"After", // continued on
	)
}

func (s *WorkerSuite) TestNewLoop(c *gc.C) {
	waitChan := make(chan struct{})
	block := make(chan struct{})
	s.clock.setAfter(4)
	count := 0
	s.cfg.Start = func() (presence.Pinger, error) {
		pinger, err := s.start()
		c.Logf("%d", count)
		if count > 3 {
			s.pinger.notify = waitChan
			s.pinger.waitBlock = block
		}
		count += 1
		return pinger, err
	}

	w, err := presence.New(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()
	defer close(block)
	<-waitChan

	s.stub.CheckCallNames(c,
		"start", "Wait", "After",
		"start", "Wait", "After",
		"start", "Wait", "After",
		"start", "Wait", "After",
		"start", "Wait",
	)
}

func (s *WorkerSuite) TestNewRetry(c *gc.C) {
	failure := errors.New("<failure>")
	s.stub.SetErrors(
		nil, nil, nil,
		failure, nil,
		failure, nil,
		nil, failure, nil,
		nil, nil,
		failure, // never reached
	)
	waitChan := make(chan struct{})
	block := make(chan struct{})
	s.clock.setAfter(5)
	delay := time.Nanosecond
	s.cfg.RetryDelay = delay
	count := 0
	s.cfg.Start = func() (presence.Pinger, error) {
		pinger, err := s.start()
		if count > 4 {
			s.pinger.notify = waitChan
			s.pinger.waitBlock = block
		}
		count += 1
		return pinger, err
	}

	w, err := presence.New(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	defer w.Stop()
	defer close(block)
	<-waitChan

	s.stub.CheckCallNames(c,
		"start", "Wait", "After",
		"start", "After",
		"start", "After",
		"start", "Wait", "After",
		"start", "Wait", "After",
		"start", "Wait",
	)
	var noWait time.Duration
	s.stub.CheckCall(c, 2, "After", noWait)
	s.stub.CheckCall(c, 4, "After", delay)
	s.stub.CheckCall(c, 6, "After", delay)
	s.stub.CheckCall(c, 9, "After", noWait)
	s.stub.CheckCall(c, 12, "After", noWait)
}

func (s *WorkerSuite) TestNewInvalidConfig(c *gc.C) {
	var cfg presence.Config

	_, err := presence.New(cfg)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

type stubPinger struct {
	*testing.Stub

	waitBlock chan struct{}
	notify    chan struct{}
}

func (s *stubPinger) Stop() error {
	s.AddCall("Stop")
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *stubPinger) Wait() error {
	s.AddCall("Wait")
	if err := s.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if s.notify != nil {
		s.notify <- struct{}{}
	}
	if s.waitBlock != nil {
		<-s.waitBlock
	}
	return nil
}

type stubClock struct {
	*coretesting.StubClock

	notify chan struct{}
}

func (s *stubClock) setAfter(numCalls int) {
	after := make(chan time.Time, numCalls)
	for i := 0; i < numCalls; i++ {
		after <- time.Now()
	}
	s.ReturnAfter = after
}

func (s *stubClock) After(d time.Duration) <-chan time.Time {
	after := s.StubClock.After(d)
	if s.notify != nil {
		s.notify <- struct{}{}
	}
	return after
}
