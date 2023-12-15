// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"
	"path"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/meterstatus"
	coretesting "github.com/juju/juju/testing"
)

const (
	AmberGracePeriod = time.Minute
	RedGracePeriod   = time.Minute * 5
)

type IsolatedWorkerConfigSuite struct {
	coretesting.BaseSuite

	config meterstatus.IsolatedConfig
}

var _ = gc.Suite(&IsolatedWorkerConfigSuite{})

func (s *IsolatedWorkerConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.config = meterstatus.IsolatedConfig{
		Runner:           struct{ meterstatus.HookRunner }{},
		StateReadWriter:  struct{ meterstatus.StateReadWriter }{},
		Clock:            struct{ meterstatus.Clock }{},
		Logger:           struct{ meterstatus.Logger }{},
		AmberGracePeriod: AmberGracePeriod,
		RedGracePeriod:   RedGracePeriod,
		TriggerFactory:   meterstatus.GetTriggers,
	}
}

func (s *IsolatedWorkerConfigSuite) TestConfigValid(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
}

func (s *IsolatedWorkerConfigSuite) TestMissingRunner(c *gc.C) {
	s.config.Runner = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Runner not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingStateReadWriter(c *gc.C) {
	s.config.StateReadWriter = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing StateReadWriter not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Clock not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingAmberGracePeriod(c *gc.C) {
	s.config.AmberGracePeriod = 0
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "amber grace period not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingRedGracePeriod(c *gc.C) {
	s.config.RedGracePeriod = 0
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "red grace period not valid")
}

func (s *IsolatedWorkerConfigSuite) TestMissingAmberEqualRed(c *gc.C) {
	s.config.RedGracePeriod = s.config.AmberGracePeriod
	err := s.config.Validate()
	c.Assert(err.Error(), gc.Equals, "amber grace period must be shorter than the red grace period")
}

type IsolatedWorkerSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir string
	clk     *testclock.Clock

	hookRan         chan struct{}
	triggersCreated chan struct{}

	worker worker.Worker
}

var _ = gc.Suite(&IsolatedWorkerSuite{})

func (s *IsolatedWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = &testing.Stub{}

	s.dataDir = c.MkDir()

	s.hookRan = make(chan struct{})
	s.triggersCreated = make(chan struct{})

	triggerFactory := func(state meterstatus.WorkerState, status string, disconectedAt time.Time, clk meterstatus.Clock, amber time.Duration, red time.Duration) (<-chan time.Time, <-chan time.Time) {
		select {
		case s.triggersCreated <- struct{}{}:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("failed to signal trigger creation")
		}
		return meterstatus.GetTriggers(state, status, disconectedAt, clk, amber, red)
	}

	s.clk = testclock.NewClock(time.Now())
	wrk, err := meterstatus.NewIsolatedStatusWorker(
		meterstatus.IsolatedConfig{
			Runner:           &stubRunner{stub: s.stub, ran: s.hookRan},
			StateReadWriter:  meterstatus.NewDiskBackedState(path.Join(s.dataDir, "meter-status.yaml")),
			Clock:            s.clk,
			Logger:           loggo.GetLogger("test"),
			AmberGracePeriod: AmberGracePeriod,
			RedGracePeriod:   RedGracePeriod,
			TriggerFactory:   triggerFactory,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk, gc.NotNil)
	s.worker = wrk
}

func (s *IsolatedWorkerSuite) TearDownTest(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *IsolatedWorkerSuite) TestTriggering(c *gc.C) {
	assertSignal(c, s.triggersCreated)

	// Wait on the red and amber timers.
	c.Assert(s.clk.WaitAdvance(AmberGracePeriod+time.Second, testing.ShortWait, 2), jc.ErrorIsNil)
	assertSignal(c, s.hookRan)

	// Don't need to ensure the timers here, we did it for both above.
	s.clk.Advance(RedGracePeriod + time.Second)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook", "RunHook")
}

// TestMissingHookError tests that errors caused by missing hooks do not stop the worker.
func (s *IsolatedWorkerSuite) TestMissingHookError(c *gc.C) {
	s.stub.SetErrors(charmrunner.NewMissingHookError("meter-status-changed"))

	assertSignal(c, s.triggersCreated)
	c.Assert(s.clk.WaitAdvance(AmberGracePeriod+time.Second, testing.ShortWait, 2), jc.ErrorIsNil)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook")
}

// TestRandomHookError tests that errors returned by hooks do not stop the worker.
func (s *IsolatedWorkerSuite) TestRandomHookError(c *gc.C) {
	s.stub.SetErrors(fmt.Errorf("blah"))

	assertSignal(c, s.triggersCreated)
	c.Assert(s.clk.WaitAdvance(AmberGracePeriod+time.Second, testing.ShortWait, 2), jc.ErrorIsNil)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook")
}
