// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	coretesting "github.com/juju/juju/internal/testing"
)

type LimiterSuite struct {
}

var _ = tc.Suite(&LimiterSuite{})

var shortAttempt = &utils.AttemptStrategy{
	Total: coretesting.ShortWait,
	Delay: 10 * time.Millisecond,
}

func (s *LimiterSuite) TestNoLimitsInvalidLimits(c *tc.C) {
	_, err := NewResourceDownloadLimiter(-1, 0)
	c.Assert(err, tc.ErrorMatches, "resource download limits must be non-negative")

	_, err = NewResourceDownloadLimiter(0, -1)
	c.Assert(err, tc.ErrorMatches, "resource download limits must be non-negative")

	_, err = NewResourceDownloadLimiter(-1, -1)
	c.Assert(err, tc.ErrorMatches, "resource download limits must be non-negative")
}

func (s *LimiterSuite) TestNoLimits(c *tc.C) {
	const totalToAcquire = 10
	limiter, err := NewResourceDownloadLimiter(0, 0)
	c.Assert(err, tc.ErrorIsNil)

	totalAcquiredCount := int32(0)
	trigger := make(chan struct{})
	started := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < totalToAcquire; i++ {
		started.Add(1)
		finished.Add(1)
		go func() {
			defer finished.Done()
			started.Done()
			limiter.Acquire(context.Background(), "app1")
			atomic.AddInt32(&totalAcquiredCount, 1)
			<-trigger
			limiter.Release("app1")
		}()
	}
	started.Wait()

	done := make(chan bool)
	go func() {
		finished.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.Fatal("finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	// All locks can be acquired.
	allLocksAcquired := false
	for a := coretesting.LongAttempt.Start(); a.Next(); a.HasNext() {
		if allLocksAcquired = atomic.LoadInt32(&totalAcquiredCount) == totalToAcquire; allLocksAcquired {
			break
		}
	}
	c.Assert(allLocksAcquired, tc.IsTrue)

	for i := 0; i < totalToAcquire; i++ {
		trigger <- struct{}{}
	}
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), tc.Equals, int32(totalToAcquire))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestGlobalLimit(c *tc.C) {
	const (
		globalLimit    = 5
		totalToAcquire = 10
	)
	limiter, err := NewResourceDownloadLimiter(globalLimit, 0)
	c.Assert(err, tc.ErrorIsNil)

	totalAcquiredCount := int32(0)
	trigger := make(chan struct{})
	started := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < totalToAcquire; i++ {
		started.Add(1)
		finished.Add(1)
		go func() {
			defer finished.Done()
			started.Done()
			limiter.Acquire(context.Background(), "app1")
			atomic.AddInt32(&totalAcquiredCount, 1)
			<-trigger
			limiter.Release("app1")
		}()
	}
	started.Wait()

	done := make(chan bool)
	go func() {
		finished.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.Fatal("finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	// We should acquire "globalLimit" locks.
	limitReached := false
	for a := coretesting.LongAttempt.Start(); a.Next(); a.HasNext() {
		if limitReached = atomic.LoadInt32(&totalAcquiredCount) == globalLimit; limitReached {
			break
		}
	}
	c.Assert(limitReached, tc.IsTrue)

	// Ensure we don't acquire more than allowed.
	for a := shortAttempt.Start(); a.Next(); a.HasNext() {
		if atomic.LoadInt32(&totalAcquiredCount) > globalLimit {
			c.Fatal("too many concurrent threads")
		}
	}

	// Allow all the locks to be acquired.
	for i := 0; i < totalToAcquire; i++ {
		trigger <- struct{}{}
	}
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), tc.Equals, int32(totalToAcquire))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestApplicationLimit(c *tc.C) {
	const (
		applicationLimit             = 5
		numApplications              = 2
		totalToAcquirePerApplication = 10
	)
	limiter, err := NewResourceDownloadLimiter(0, applicationLimit)
	c.Assert(err, tc.ErrorIsNil)

	totalAcquiredCount := int32(0)
	trigger := make(chan struct{})
	started := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < numApplications*totalToAcquirePerApplication; i++ {
		started.Add(1)
		finished.Add(1)
		uuid := "app1"
		if i >= totalToAcquirePerApplication {
			uuid = "app2"
		}
		go func(uui string) {
			defer finished.Done()
			started.Done()
			limiter.Acquire(context.Background(), uuid)
			atomic.AddInt32(&totalAcquiredCount, 1)
			<-trigger
			limiter.Release(uuid)
		}(uuid)
	}
	started.Wait()

	done := make(chan bool)
	go func() {
		finished.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.Fatal("finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	// We should acquire 2 * "applicationLimit" locks.
	limitReached := false
	for a := coretesting.LongAttempt.Start(); a.Next(); a.HasNext() {
		if limitReached = atomic.LoadInt32(&totalAcquiredCount) == numApplications*applicationLimit; limitReached {
			break
		}
	}
	c.Logf("got %d", totalAcquiredCount)
	c.Assert(limitReached, tc.IsTrue)

	// Ensure we don't acquire more than allowed.
	for a := shortAttempt.Start(); a.Next(); a.HasNext() {
		if atomic.LoadInt32(&totalAcquiredCount) > numApplications*applicationLimit {
			c.Fatal("too many concurrent threads")
		}
	}

	// Allow all the locks to be acquired.
	for i := 0; i < numApplications*totalToAcquirePerApplication; i++ {
		trigger <- struct{}{}
	}
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), tc.Equals, int32(numApplications*totalToAcquirePerApplication))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestGlobalAndApplicationLimit(c *tc.C) {
	const (
		globalLimit                  = 5
		applicationLimit             = 3
		numApplications              = 3
		totalToAcquirePerApplication = 2
	)
	limiter, err := NewResourceDownloadLimiter(globalLimit, applicationLimit)
	c.Assert(err, tc.ErrorIsNil)

	totalAcquiredCount := int32(0)
	trigger := make(chan struct{})
	started := sync.WaitGroup{}
	finished := sync.WaitGroup{}
	for i := 0; i < numApplications*totalToAcquirePerApplication; i++ {
		started.Add(1)
		finished.Add(1)
		uuid := "app1"
		if i >= 2*totalToAcquirePerApplication {
			uuid = "app3"
		} else if i >= totalToAcquirePerApplication {
			uuid = "app2"
		}
		go func(uui string) {
			defer finished.Done()
			started.Done()
			limiter.Acquire(context.Background(), uuid)
			atomic.AddInt32(&totalAcquiredCount, 1)
			<-trigger
			limiter.Release(uuid)
		}(uuid)
	}
	started.Wait()

	done := make(chan bool)
	go func() {
		finished.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.Fatal("finished too soon")
	case <-time.After(coretesting.ShortWait):
	}

	// We should acquire "globalLimit" locks as that is less
	// than the total number of application locks.
	limitReached := false
	for a := coretesting.LongAttempt.Start(); a.Next(); a.HasNext() {
		if limitReached = atomic.LoadInt32(&totalAcquiredCount) == globalLimit; limitReached {
			break
		}
	}
	c.Assert(limitReached, tc.IsTrue)

	// Ensure we don't acquire more than allowed.
	for a := shortAttempt.Start(); a.Next(); a.HasNext() {
		if atomic.LoadInt32(&totalAcquiredCount) > globalLimit {
			c.Fatal("too many concurrent threads")
		}
	}

	// Allow all the locks to be acquired.
	for i := 0; i < numApplications*totalToAcquirePerApplication; i++ {
		trigger <- struct{}{}
	}
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), tc.Equals, int32(numApplications*totalToAcquirePerApplication))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}
