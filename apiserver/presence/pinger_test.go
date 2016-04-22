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
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.pinger = &stubPinger{Stub: s.stub}
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

type stubPinger struct {
	*testing.Stub
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
	return nil
}
