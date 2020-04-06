// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"
	"path"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/meterstatus/mocks"
)

const (
	AmberGracePeriod = time.Minute
	RedGracePeriod   = time.Minute * 5
)

type IsolatedWorkerConfigSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir string
}

var _ = gc.Suite(&IsolatedWorkerConfigSuite{})

func (s *IsolatedWorkerConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.dataDir = c.MkDir()
}

func (s *IsolatedWorkerConfigSuite) TestConfigValidation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tests := []struct {
		cfg      meterstatus.IsolatedConfig
		expected string
	}{{
		cfg: meterstatus.IsolatedConfig{
			Runner:          &stubRunner{stub: s.stub},
			StateReadWriter: mocks.NewMockStateReadWriter(ctrl),
		},
		expected: "clock not provided",
	}, {
		cfg: meterstatus.IsolatedConfig{
			Clock:           testclock.NewClock(time.Now()),
			StateReadWriter: mocks.NewMockStateReadWriter(ctrl),
		},
		expected: "hook runner not provided",
	}, {
		cfg: meterstatus.IsolatedConfig{
			Clock:  testclock.NewClock(time.Now()),
			Runner: &stubRunner{stub: s.stub},
		},
		expected: "state read/writer not provided",
	}}
	for i, test := range tests {
		c.Logf("running test %d", i)
		err := test.cfg.Validate()
		c.Assert(err, gc.ErrorMatches, test.expected)
	}
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

	triggerFactory := func(state meterstatus.WorkerState, status string, disconectedAt time.Time, clk clock.Clock, amber time.Duration, red time.Duration) (<-chan time.Time, <-chan time.Time) {
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
			AmberGracePeriod: AmberGracePeriod,
			RedGracePeriod:   RedGracePeriod,
			TriggerFactory:   triggerFactory,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wrk, gc.NotNil)
	s.worker = wrk
}

func (s *IsolatedWorkerSuite) TearDownTest(c *gc.C) {
	s.worker.Kill()
	err := s.worker.Wait()
	c.Assert(err, jc.ErrorIsNil)
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
