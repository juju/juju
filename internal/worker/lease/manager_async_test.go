// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/lease"
)

type leaseMap = map[corelease.Key]corelease.Info

// AsyncSuite checks that expiries and claims that block don't prevent
// subsequent updates.
type AsyncSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&AsyncSuite{})

func (s *AsyncSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.lease")
	logger.SetLogLevel(loggo.TRACE)
	logger = loggo.GetLogger("lease_test")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *AsyncSuite) TestRevokeTimeout(c *tc.C) {
	// When a timeout happens on revoke we retry.
	revokeCalls := make(chan struct{})
	fix := Fixture{
		leases: leaseMap{
			key("requiem"): {
				Holder: "verdi",
				Expiry: offset(time.Minute),
			},
		},
		expectCalls: []call{{
			method: "RevokeLease",
			args:   []interface{}{key("requiem"), "verdi"},
			err:    corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case revokeCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out sending revoke")
				}
			},
		}, {
			method: "RevokeLease",
			args:   []interface{}{key("requiem"), "verdi"},
			callback: func(leases leaseMap) {
				delete(leases, key("requiem"))
				close(revokeCalls)
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		revoker, err := manager.Revoker("namespace", "modelUUID")
		c.Assert(err, tc.ErrorIsNil)

		result := make(chan error)
		go func() {
			result <- revoker.Revoke("requiem", "verdi")
		}()

		select {
		case <-revokeCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for revoke")
		}

		// Two waiters:
		// - one is the nextTick timer, set for 1 minute in the future
		// - two is the claim retry timer
		err = clock.WaitAdvance(50*time.Millisecond, coretesting.LongWait, 2)
		c.Assert(err, tc.ErrorIsNil)

		select {
		case err := <-result:
			c.Assert(err, tc.ErrorIsNil)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response")
		}
	})
}

func (s *AsyncSuite) TestRevokeRepeatedTimeout(c *tc.C) {
	// When a timeout happens on revoke we retry - if we hit the retry
	// limit we should kill the manager.
	revokeCalls := make(chan struct{})

	var calls []call
	for i := 0; i < lease.MaxRetries; i++ {
		calls = append(calls, call{
			method: "RevokeLease",
			args:   []interface{}{key("requiem"), "verdi"},
			err:    corelease.ErrTimeout,
			callback: func(_ leaseMap) {
				select {
				case revokeCalls <- struct{}{}:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out sending revoke")
				}
			},
		})
	}
	fix := Fixture{
		leases: leaseMap{
			key("requiem"): {
				Holder: "verdi",
				Expiry: offset(time.Minute),
			},
		},
		expectCalls: calls,
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		result := make(chan error)
		revoker, err := manager.Revoker("namespace", "modelUUID")
		c.Assert(err, tc.ErrorIsNil)
		go func() {
			result <- revoker.Revoke("requiem", "verdi")
		}()

		duration := lease.InitialRetryDelay
		for i := 0; i < lease.MaxRetries-1; i++ {
			c.Logf("retry %d", i)
			select {
			case <-revokeCalls:
			case <-result:
				c.Fatalf("got result too soon")
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for revoke call")
			}

			// There should be 2 waiters:
			//  - nextTick has a timer once things expire
			//  - retryingClaim has an attempt timer
			c.Assert(clock.WaitAdvance(duration, coretesting.LongWait, 2), tc.ErrorIsNil)
			duration = time.Duration(float64(duration)*lease.RetryBackoffFactor + 1)
		}

		select {
		case <-revokeCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for final revoke call")
		}

		select {
		case err := <-result:
			c.Assert(errors.Cause(err), tc.Equals, corelease.ErrTimeout)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		workertest.CheckAlive(c, manager)
	})
}

func (s *AsyncSuite) TestClaimSlow(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)

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
		c.Assert(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 1), tc.ErrorIsNil)

		// We should be able to get the response for the second claim
		// even though the first hasn't come back yet.
		select {
		case err := <-response2:
			c.Assert(err, tc.ErrorIsNil)
		case <-response1:
			c.Fatalf("response1 was ready")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response2")
		}

		close(slowFinish)

		c.Assert(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 1), tc.ErrorIsNil)

		// Now response1 should come back.
		select {
		case err := <-response1:
			c.Assert(errors.Cause(err), tc.Equals, corelease.ErrClaimDenied)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response1")
		}
	})
}

func (s *AsyncSuite) TestClaimTwoErrors(c *tc.C) {
	oneStarted := make(chan struct{})
	oneFinish := make(chan struct{})
	twoStarted := make(chan struct{})
	twoFinish := make(chan struct{})

	fix := Fixture{
		expectDirty: true,
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				key("one"),
				corelease.Request{"terry", time.Minute},
			},
			err: errors.New("terry is bad"),
			parallelCallback: func(mu *sync.Mutex, leases leaseMap) {
				close(oneStarted)
				select {
				case <-oneFinish:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out waiting for oneFinish")
				}
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				key("two"),
				corelease.Request{"lance", time.Minute},
			},
			err: errors.New("lance is also bad"),
			parallelCallback: func(mu *sync.Mutex, leases leaseMap) {
				close(twoStarted)
				select {
				case <-twoFinish:
				case <-time.After(coretesting.LongWait):
					c.Errorf("timed out waiting for twoFinish")
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, tc.ErrorIsNil)

		response1 := make(chan error)
		go func() {
			response1 <- claimer.Claim("one", "terry", time.Minute)
		}()
		select {
		case <-oneStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for oneStarted")
		}

		response2 := make(chan error)
		go func() {
			response2 <- claimer.Claim("two", "lance", time.Minute)
		}()

		select {
		case <-twoStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for twoStarted")
		}

		// By now, both of the claims have had their processing started
		// by the store, so the lease manager will have two elements
		// in the wait group.
		close(oneFinish)
		// We should be able to get error responses from both of them.
		select {
		case err1 := <-response1:
			c.Check(err1, tc.ErrorMatches, "lease manager stopped")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response2")
		}

		close(twoFinish)
		select {
		case err2 := <-response2:
			c.Check(err2, tc.ErrorMatches, "lease manager stopped")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response2")
		}

		// Since we unblock one before two, we know the error from
		// the manager is bad terry
		err = workertest.CheckKilled(c, manager)
		c.Assert(err, tc.ErrorMatches, "terry is bad")
	})
}

func (s *AsyncSuite) TestClaimTimeout(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
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
		c.Assert(err, tc.ErrorIsNil)

		select {
		case err := <-result:
			c.Assert(err, tc.ErrorIsNil)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for response")
		}
	})
}

func (s *AsyncSuite) TestClaimNoticesEarlyExpiry(c *tc.C) {
	fix := Fixture{
		leases: leaseMap{
			key("dmdc"): {
				Holder: "terry",
				Expiry: offset(10 * time.Minute),
			},
		},
		expectCalls: []call{{
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
		}, {
			method: "ClaimLease",
			args: []interface{}{
				key("fudge"),
				corelease.Request{"chocolate", time.Minute},
			},
			callback: func(leases leaseMap) {
				leases[key("fudge")] = corelease.Info{
					Holder: "chocolate",
					Expiry: offset(2 * time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// When we first start, we should not yet expire because the
		// Expiry should be 10 minutes into the future. But the first claim
		// will create an entry that expires in only 1 minute, so we should
		// reset our expire timeout
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, tc.ErrorIsNil)
		err = claimer.Claim("icecream", "rosie", time.Minute)
		c.Assert(err, tc.ErrorIsNil)
		// We sleep for 30s which *shouldn't* trigger any Expiry. And then we get
		// another claim that also wants 1 minute duration. But that should not cause the
		// timer to wake up in 1minute, but the 30s that are remaining.
		c.Assert(clock.WaitAdvance(30*time.Second, testing.LongWait, 1), tc.ErrorIsNil)
		// The second claim tries to set a timeout of another minute, but that should
		// not cause the timer to get reset any later than it already is.
		// Chocolate is also given a slightly longer timeout (2min after epoch)
		err = claimer.Claim("fudge", "chocolate", time.Minute)
		c.Assert(err, tc.ErrorIsNil)
		// Now when we advance the clock another 30s, it should wake up and
		// expire "icecream", and then queue up that we should expire "fudge"
		// 1m later
		c.Assert(clock.WaitAdvance(30*time.Second, testing.LongWait, 1), tc.ErrorIsNil)
	})
}

func (s *AsyncSuite) TestClaimRepeatedTimeout(c *tc.C) {
	// When a claim times out too many times we give up.
	claimCalls := make(chan struct{})
	var calls []call
	for i := 0; i < lease.MaxRetries; i++ {
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
		c.Assert(err, tc.ErrorIsNil)
		go func() {
			result <- claimer.Claim("icecream", "rosie", time.Minute)
		}()

		duration := lease.InitialRetryDelay
		for i := 0; i < lease.MaxRetries-1; i++ {
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
			c.Assert(clock.WaitAdvance(duration, coretesting.LongWait, 2), tc.ErrorIsNil)
			duration = time.Duration(float64(duration)*lease.RetryBackoffFactor + 1)
		}

		select {
		case <-claimCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for final claim call")
		}

		select {
		case err := <-result:
			c.Assert(errors.Cause(err), tc.Equals, corelease.ErrTimeout)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		workertest.CheckAlive(c, manager)
	})
}

func (s *AsyncSuite) TestClaimRepeatedInvalid(c *tc.C) {
	// When a claim is invalid for too long, we give up
	claimCalls := make(chan struct{})
	var calls []call
	for i := 0; i < lease.MaxRetries; i++ {
		calls = append(calls, call{
			method: "ClaimLease",
			args: []interface{}{
				key("icecream"),
				corelease.Request{"rosie", time.Minute},
			},
			err: corelease.ErrInvalid,
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
		c.Assert(err, tc.ErrorIsNil)
		go func() {
			result <- claimer.Claim("icecream", "rosie", time.Minute)
		}()

		duration := lease.InitialRetryDelay
		for i := 0; i < lease.MaxRetries-1; i++ {
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
			c.Assert(clock.WaitAdvance(duration, coretesting.LongWait, 2), tc.ErrorIsNil)
			duration = time.Duration(float64(duration)*lease.RetryBackoffFactor + 1)
		}

		select {
		case <-claimCalls:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for final claim call")
		}

		select {
		case err := <-result:
			c.Assert(errors.Cause(err), tc.Equals, corelease.ErrClaimDenied)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		workertest.CheckAlive(c, manager)
	})
}

func (s *AsyncSuite) TestWaitsForGoroutines(c *tc.C) {
	// The manager should wait for all of its child expire and claim
	// goroutines to be finished before it stops.
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

		result := make(chan error)
		claimer, err := manager.Claimer("namespace", "modelUUID")
		c.Assert(err, tc.ErrorIsNil)
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
			c.Assert(err, tc.ErrorMatches, "lease manager stopped")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for result")
		}

		err = workertest.CheckKilled(c, manager)
		c.Assert(err, tc.ErrorIsNil)
	})
}
