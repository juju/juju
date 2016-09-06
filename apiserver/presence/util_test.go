// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/presence"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/workertest"
)

var (
	fiveSeconds       = 5 * time.Second
	almostFiveSeconds = fiveSeconds - time.Nanosecond
)

// Context exposes useful functionality to fixture tests.
type Context interface {

	// WaitPinger() returns the first pinger started by the SUT that
	// has not already been returned from this method.
	WaitPinger() worker.Worker

	// WaitAlarms() returns once the SUT has set (but not
	// necessarily responded to) N alarms (e.g. calls to
	// clock.After).
	WaitAlarms(int)

	// AdvanceClock() advances the SUT's clock by the duration. If
	// you're testing alarms, be sure that you've waited for the
	// relevant alarm to be set before you advance the clock.
	AdvanceClock(time.Duration)
}

// FixtureTest is called with a Context and a running Worker.
type FixtureTest func(Context, *presence.Worker)

func NewFixture(errors ...error) *Fixture {
	return &Fixture{errors}
}

// Fixture makes it easy to manipulate a running worker's environment
// and test its behaviour in response.
type Fixture struct {
	errors []error
}

// Run runs test against a fresh Stub, which is returned to the client
// for further analysis.
func (fix *Fixture) Run(c *gc.C, test FixtureTest) *testing.Stub {
	stub := &testing.Stub{}
	stub.SetErrors(fix.errors...)
	run(c, stub, test)
	return stub
}

func run(c *gc.C, stub *testing.Stub, test FixtureTest) {
	context := &context{
		c:       c,
		stub:    stub,
		clock:   testing.NewClock(time.Now()),
		timeout: time.After(time.Second),
		starts:  make(chan worker.Worker, 1000),
	}
	defer context.checkCleanedUp()

	worker, err := presence.New(presence.Config{
		Identity:   names.NewMachineTag("1"),
		Start:      context.startPinger,
		Clock:      context.clock,
		RetryDelay: fiveSeconds,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	test(context, worker)
}

// context implements Context.
type context struct {
	c       *gc.C
	stub    *testing.Stub
	clock   *testing.Clock
	timeout <-chan time.Time

	starts  chan worker.Worker
	mu      sync.Mutex
	current worker.Worker
}

// WaitPinger is part of the Context interface.
func (context *context) WaitPinger() worker.Worker {
	context.c.Logf("waiting for pinger...")
	select {
	case pinger := <-context.starts:
		return pinger
	case <-context.timeout:
		context.c.Fatalf("timed out waiting for pinger")
		return nil
	}
}

// WaitAlarms is part of the Context interface.
func (context *context) WaitAlarms(count int) {
	context.c.Logf("waiting for %d alarms...", count)
	for i := 0; i < count; i++ {
		select {
		case <-context.clock.Alarms():
		case <-context.timeout:
			context.c.Fatalf("timed out waiting for alarm %d", i)
		}
	}
}

// AdvanceClock is part of the Context interface.
func (context *context) AdvanceClock(d time.Duration) {
	context.clock.Advance(d)
}

func (context *context) startPinger() (presence.Pinger, error) {
	context.stub.AddCall("Start")
	context.checkCleanedUp()
	if startErr := context.stub.NextErr(); startErr != nil {
		return nil, startErr
	}

	context.mu.Lock()
	defer context.mu.Unlock()
	pingerErr := context.stub.NextErr()
	context.current = workertest.NewErrorWorker(pingerErr)
	context.starts <- context.current
	return mockPinger{context.current}, nil
}

func (context *context) checkCleanedUp() {
	context.c.Logf("checking no active current pinger")
	context.mu.Lock()
	defer context.mu.Unlock()
	if context.current != nil {
		workertest.CheckKilled(context.c, context.current)
	}
}

// mockPinger implements presence.Pinger for the convenience of the
// tests.
type mockPinger struct {
	worker.Worker
}

func (mock mockPinger) Stop() error {
	return worker.Stop(mock.Worker)
}

func (mock mockPinger) Wait() error {
	return mock.Worker.Wait()
}

// validConfig returns a presence.Config that will validate, but fail
// violently if actually used for anything.
func validConfig() presence.Config {
	return presence.Config{
		Identity:   struct{ names.Tag }{},
		Start:      func() (presence.Pinger, error) { panic("no") },
		Clock:      struct{ clock.Clock }{},
		RetryDelay: time.Nanosecond,
	}
}

func checkInvalid(c *gc.C, config presence.Config, message string) {
	check := func(err error) {
		c.Check(err, gc.ErrorMatches, message)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}

	err := config.Validate()
	check(err)

	worker, err := presence.New(config)
	if !c.Check(worker, gc.IsNil) {
		workertest.CleanKill(c, worker)
	}
	check(err)
}
