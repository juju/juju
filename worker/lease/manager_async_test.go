// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
)

type leaseMap = map[corelease.Key]corelease.Info

// AsyncSuite checks that expiries and claims that block don't prevent
// subsequent updates.
type AsyncSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AsyncSuite{})

func (s *AsyncSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.lease")
	logger.SetLogLevel(loggo.TRACE)
	logger = loggo.GetLogger("lease_test")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *AsyncSuite) TestExpirySlow(c *gc.C) {
	// Ensure that even if an expiry is taking a long time, another
	// expiry after it can still work.

	slowStarted := make(chan struct{})
	slowFinish := make(chan struct{})

	quickFinished := make(chan struct{})

	fix := Fixture{
		leases: leaseMap{
			key("thing1"): {
				Holder: "holden",
				Expiry: offset(-time.Second),
			},
			key("thing2"): {
				Holder: "miller",
				Expiry: offset(time.Second),
			},
		},

		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("thing1")},
			err:    corelease.ErrInvalid,
			parallelCallback: func(mu *sync.Mutex, leases leaseMap) {
				mu.Lock()
				delete(leases, key("thing1"))
				mu.Unlock()

				select {
				case slowStarted <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out sending slowStarted")
				}
				select {
				case <-slowFinish:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out waiting for slowFinish")
				}

			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("thing2")},
			callback: func(leases leaseMap) {
				delete(leases, key("thing2"))
				close(quickFinished)
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		select {
		case <-slowStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for slowStarted")
		}
		// The Waiter here should be the Clock.After in tick() that is waiting
		// for Expire to try to cleanup. But it should eventually skip even if
		// it is blocking
		c.Assert(clock.WaitAdvance(50*time.Millisecond, coretesting.LongWait, 1), jc.ErrorIsNil)
		// The next waiter will be the main loop, noticing that the next time to expire is
		// in 1s
		c.Assert(clock.WaitAdvance(time.Second-50*time.Millisecond, coretesting.LongWait, 1), jc.ErrorIsNil)

		select {
		case <-quickFinished:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for quickFinished")
		}

		close(slowFinish)

	})
}

func (s *AsyncSuite) TestExpiryTimeout(c *gc.C) {
	// When a timeout happens on expiry we retry.
	expireCalls := make(chan struct{})
	fix := Fixture{
		leases: leaseMap{
			key("requiem"): {
				Holder: "verdi",
				Expiry: offset(-time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("requiem")},
			err:    corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case expireCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out sending expired")
				}
			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("requiem")},
			callback: func(leases leaseMap) {
				delete(leases, key("requiem"))
				close(expireCalls)
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		select {
		case <-expireCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for 1st expireCall")
		}

		// We want two waiters - one for the main loop, and one for
		// the retry delay.
		err := clock.WaitAdvance(50*time.Millisecond, coretesting.LongWait, 2)
		c.Assert(err, jc.ErrorIsNil)

		select {
		case _, ok := <-expireCalls:
			c.Assert(ok, gc.Equals, false)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for 2nd expireCall")
		}
	})
}

func (s *AsyncSuite) TestExpiryRepeatedTimeout(c *gc.C) {
	// When a timeout happens on expiry we retry - if we hit the retry
	// limit we should kill the manager.
	expireCalls := make(chan struct{})

	var calls []call
	for i := 0; i < 5; i++ {
		calls = append(calls,
			call{method: "Refresh"},
			call{
				method: "ExpireLease",
				args:   []interface{}{key("requiem")},
				err:    corelease.ErrTimeout,
				callback: func(_ leaseMap) {
					select {
					case expireCalls <- struct{}{}:
					case <-time.After(coretesting.LongWait):
						c.Fatalf("timed out sending expired")
					}
				},
			},
		)
	}
	fix := Fixture{
		leases: leaseMap{
			key("requiem"): {
				Holder: "mozart",
				Expiry: offset(-time.Second),
			},
		},
		expectCalls: calls,
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		select {
		case <-expireCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for 1st expireCall")
		}

		delay := 50 * time.Millisecond
		for i := 0; i < 4; i++ {
			c.Logf("retry %d", i+1)
			// Two timers:
			// - nextTick timer
			// - retryingExpiry timers
			err := clock.WaitAdvance(delay, coretesting.LongWait, 2)
			c.Assert(err, jc.ErrorIsNil)
			select {
			case <-expireCalls:
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for expireCall")
			}
			delay *= 2
		}
		workertest.CheckAlive(c, manager)
	})
}

func (s *AsyncSuite) TestExpiryInterruptedRetry(c *gc.C) {
	// Check that retries are stopped when the manager is killed.
	expireCalls := make(chan struct{})
	fix := Fixture{
		leases: leaseMap{
			key("requiem"): {
				Holder: "fauré",
				Expiry: offset(-time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("requiem")},
			err:    corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case expireCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out sending expired")
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		select {
		case <-expireCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for 1st expireCall")
		}

		// Ensure the main loop and the retry loop are both waiting
		// for the clock without advancing it.
		err := clock.WaitAdvance(0, coretesting.LongWait, 2)
		c.Assert(err, jc.ErrorIsNil)

		// Stopping the worker should cancel the retry.
		err = worker.Stop(manager)
		c.Assert(err, jc.ErrorIsNil)

		// Advance the clock to trigger the next retry if it's
		// waiting.
		err = clock.WaitAdvance(50*time.Millisecond, coretesting.ShortWait, 2)
		c.Assert(err, jc.ErrorIsNil)

		// Allow some wallclock time for a non-cancelled retry to
		// happen if stopping the worker didn't cancel it. This is not
		// ideal but I can't see a better way to verify that the retry
		// doesn't happen - adding an exploding call to expectCalls
		// makes the store wait for that call to be made. This is
		// verified to pass reliably if the retry gets cancelled and
		// fail reliably otherwise.
		time.Sleep(coretesting.ShortWait)
	})
}

func (s *AsyncSuite) TestClaimSlow(c *gc.C) {
	slowStarted := make(chan struct{})
	slowFinish := make(chan struct{})

	fix := Fixture{
		leases: leaseMap{
			key("dmdc"): {
				Holder: "terry",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				key("dmdc"),
				corelease.Request{"terry", time.Minute},
			},
			err: corelease.ErrInvalid,
			parallelCallback: func(mu *sync.Mutex, leases leaseMap) {
				select {
				case slowStarted <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out sending slowStarted")
				}
				select {
				case <-slowFinish:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out waiting for slowFinish")
				}
				mu.Lock()
				leases[key("dmdc")] = corelease.Info{
					Holder: "lance",
					Expiry: offset(time.Minute),
				}
				mu.Unlock()
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				key("antiquisearchers"),
				corelease.Request{"art", time.Minute},
			},
			callback: func(leases leaseMap) {
				leases[key("antiquisearchers")] = corelease.Info{
					Holder: "art",
					Expiry: offset(time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, jc.ErrorIsNil)

		response1 := make(chan error)
		go func() {
			response1 <- claimer.Claim("dmdc", "terry", time.Minute)
		}()

		select {
		case <-slowStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for slowStarted")
		}
		response2 := make(chan error)
		go func() {
			response2 <- claimer.Claim("antiquisearchers", "art", time.Minute)
		}()

		// response1 should have failed its claim, and now be waiting to retry
		// only 1 waiter, which is the 'when should we expire next' timer.
		c.Assert(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 1), jc.ErrorIsNil)

		// We should be able to get the response for the second claim
		// even though the first hasn't come back yet.
		select {
		case err := <-response2:
			c.Assert(err, jc.ErrorIsNil)
		case <-response1:
			c.Fatalf("response1 was ready")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response2")
		}

		close(slowFinish)

		c.Assert(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 1), jc.ErrorIsNil)

		// Now response1 should come back.
		select {
		case err := <-response1:
			c.Assert(errors.Cause(err), gc.Equals, corelease.ErrClaimDenied)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response1")
		}
	})
}

func (s *AsyncSuite) TestClaimTimeout(c *gc.C) {
	// When a claim times out we retry.
	claimCalls := make(chan struct{})
	fix := Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				key("icecream"),
				corelease.Request{"rosie", time.Minute},
			},
			err: corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case claimCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out sending claim")
				}
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				key("icecream"),
				corelease.Request{"rosie", time.Minute},
			},
			callback: func(leases leaseMap) {
				leases[key("icecream")] = corelease.Info{
					Holder: "rosie",
					Expiry: offset(time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		result := make(chan error)
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, jc.ErrorIsNil)
		go func() {
			result <- claimer.Claim("icecream", "rosie", time.Minute)
		}()

		select {
		case <-claimCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for claim")
		}

		// Two waiters:
		// - one is the nextTick timer, set for 1 minute in the future
		// - two is the claim retry timer
		err = clock.WaitAdvance(50*time.Millisecond, coretesting.LongWait, 2)

		select {
		case err := <-result:
			c.Assert(err, jc.ErrorIsNil)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response")
		}
	})
}

func (s *AsyncSuite) TestClaimRepeatedTimeout(c *gc.C) {
	// When a claim times out too many times we give up.
	claimCalls := make(chan struct{})
	var calls []call
	for i := 0; i < 5; i++ {
		calls = append(calls, call{
			method: "ClaimLease",
			args: []interface{}{
				key("icecream"),
				corelease.Request{"rosie", time.Minute},
			},
			err: corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case claimCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out sending claim")
				}
			},
		})
	}
	fix := Fixture{
		expectCalls: calls,
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		result := make(chan error)
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, jc.ErrorIsNil)
		go func() {
			result <- claimer.Claim("icecream", "rosie", time.Minute)
		}()

		duration := 50 * time.Millisecond
		for i := 0; i < 4; i++ {
			c.Logf("retry %d", i)
			select {
			case <-claimCalls:
			case <-result:
				c.Fatalf("got result too soon")
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for claim call")
			}

			// There should be 2 waiters:
			//  - nextTick has a timer once things expire
			//  - retryingClaim has an attempt timer
			err := clock.WaitAdvance(duration, coretesting.LongWait, 2)
			c.Assert(err, jc.ErrorIsNil)
			duration *= 2
		}

		select {
		case <-claimCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for final claim call")
		}

		select {
		case err := <-result:
			c.Assert(errors.Cause(err), gc.Equals, corelease.ErrTimeout)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		workertest.CheckAlive(c, manager)
	})
}

func (s *AsyncSuite) TestWaitsForGoroutines(c *gc.C) {
	// The manager should wait for all of its child expire and claim
	// goroutines to be finished before it stops.
	tickStarted := make(chan struct{})
	tickFinish := make(chan struct{})
	claimStarted := make(chan struct{})
	claimFinish := make(chan struct{})
	fix := Fixture{
		leases: leaseMap{
			key("legacy"): {
				Holder: "culprate",
				Expiry: offset(-time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("legacy")},
			parallelCallback: func(_ *sync.Mutex, _ leaseMap) {
				close(tickStarted)
				// Block until asked to stop.
				<-tickFinish
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				key("blooadoath"),
				corelease.Request{"hand", time.Minute},
			},
			parallelCallback: func(_ *sync.Mutex, _ leaseMap) {
				close(claimStarted)
				<-claimFinish
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		select {
		case <-tickStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for expire start")
		}

		result := make(chan error)
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, jc.ErrorIsNil)
		go func() {
			result <- claimer.Claim("blooadoath", "hand", time.Minute)
		}()

		// Ensure we've called claim in the store and are waiting for
		// a response.
		select {
		case <-claimStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for claim start")
		}

		// If we kill the manager now it won't finish until the claim
		// call finishes (no worries about timeouts because we aren't
		// advancing the test clock).
		manager.Kill()
		workertest.CheckAlive(c, manager)

		// Now if we finish the claim, the result comes back.
		close(claimFinish)

		select {
		case err := <-result:
			c.Assert(err, gc.ErrorMatches, "lease manager stopped")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		workertest.CheckAlive(c, manager)

		// And when we finish the expire the worker stops.
		close(tickFinish)

		err = workertest.CheckKilled(c, manager)
		c.Assert(err, jc.ErrorIsNil)
	})
}
