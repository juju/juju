// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"fmt"
	"path"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/uniter/runner/context"
)

const (
	AmberGracePeriod = time.Minute
	RedGracePeriod   = time.Minute * 5
)

type IsolatedWorkerSuite struct {
	coretesting.BaseSuite

	stub *testing.Stub

	dataDir string
	lock    *fslock.Lock

	clk *coretesting.Clock

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

	s.clk = coretesting.NewClock(time.Now())
	wrk, err := meterstatus.NewIsolatedStatusWorker(
		meterstatus.IsolatedConfig{
			Runner:           &stubRunner{stub: s.stub, ran: s.hookRan},
			StateFile:        meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
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

func (s *IsolatedWorkerSuite) TestConfigValidation(c *gc.C) {
	tests := []struct {
		cfg      meterstatus.IsolatedConfig
		expected string
	}{{
		cfg: meterstatus.IsolatedConfig{
			Runner:    &stubRunner{stub: s.stub},
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
		},
		expected: "clock not provided",
	}, {
		cfg: meterstatus.IsolatedConfig{
			Clock:     coretesting.NewClock(time.Now()),
			StateFile: meterstatus.NewStateFile(path.Join(s.dataDir, "meter-status.yaml")),
		},
		expected: "hook runner not provided",
	}, {
		cfg: meterstatus.IsolatedConfig{
			Clock:  coretesting.NewClock(time.Now()),
			Runner: &stubRunner{stub: s.stub},
		},
		expected: "state file not provided",
	}}
	for i, test := range tests {
		c.Logf("running test %d", i)
		err := test.cfg.Validate()
		c.Assert(err, gc.ErrorMatches, test.expected)
	}
}

func (s *IsolatedWorkerSuite) TestTriggering(c *gc.C) {
	assertSignal(c, s.triggersCreated)
	s.clk.Advance(AmberGracePeriod + time.Second)
	assertSignal(c, s.hookRan)
	s.clk.Advance(RedGracePeriod + time.Second)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook", "RunHook")
}

// TestMissingHookError tests that errors caused by missing hooks do not stop the worker.
func (s *IsolatedWorkerSuite) TestMissingHookError(c *gc.C) {
	s.stub.SetErrors(context.NewMissingHookError("meter-status-changed"))

	assertSignal(c, s.triggersCreated)
	s.clk.Advance(AmberGracePeriod + time.Second)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook")
}

// TestRandomHookError tests that errors returned by hooks do not stop the worker.
func (s *IsolatedWorkerSuite) TestRandomHookError(c *gc.C) {
	s.stub.SetErrors(fmt.Errorf("blah"))

	assertSignal(c, s.triggersCreated)
	s.clk.Advance(AmberGracePeriod + time.Second)
	assertSignal(c, s.hookRan)

	s.stub.CheckCallNames(c, "RunHook")
}
