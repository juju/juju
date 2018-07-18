// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	logger := loggo.GetLogger("juju.worker.lease")
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
					c.Fatalf("timed out sending slowStarted")
				}
				select {
				case <-slowFinish:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out waiting for slowFinish")
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
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		select {
		case <-slowStarted:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for slowStarted")
		}
		err := clock.WaitAdvance(time.Second, coretesting.LongWait, 1)
		c.Assert(err, jc.ErrorIsNil)

		select {
		case <-quickFinished:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for quickFinished")
		}

		close(slowFinish)

	})
}

func (s *AsyncSuite) TestExpiryTimeout(c *gc.C) {
	c.Fatalf("writeme")
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
					c.Fatalf("timed out sending slowStarted")
				}
				select {
				case <-slowFinish:
				case <-time.After(coretesting.LongWait):
					c.Fatalf("timed out waiting for slowFinish")
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
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
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
	c.Fatalf("writeme")
}
