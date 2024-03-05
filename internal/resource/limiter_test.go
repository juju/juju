// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"sync"
	"sync/atomic"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/resource"
	coretesting "github.com/juju/juju/testing"
)

type LimiterSuite struct {
}

var _ = gc.Suite(&LimiterSuite{})

var shortAttempt = &utils.AttemptStrategy{
	Total: coretesting.ShortWait,
	Delay: 10 * time.Millisecond,
}

func (s *LimiterSuite) TestNoLimits(c *gc.C) {

	const totalToAcquire = 10
	limiter := resource.NewResourceDownloadLimiter(0, 0)

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
			limiter.Acquire("app1")
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
	c.Assert(allLocksAcquired, jc.IsTrue)

	for i := 0; i < totalToAcquire; i++ {
		trigger <- struct{}{}
	}
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), gc.Equals, int32(totalToAcquire))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestGlobalLimit(c *gc.C) {

	const (
		globalLimit    = 5
		totalToAcquire = 10
	)
	limiter := resource.NewResourceDownloadLimiter(globalLimit, 0)

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
			limiter.Acquire("app1")
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
	c.Assert(limitReached, jc.IsTrue)

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
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), gc.Equals, int32(totalToAcquire))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestApplicationLimit(c *gc.C) {

	const (
		applicationLimit             = 5
		numApplications              = 2
		totalToAcquirePerApplication = 10
	)
	limiter := resource.NewResourceDownloadLimiter(0, applicationLimit)

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
			limiter.Acquire(uuid)
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
	c.Assert(limitReached, jc.IsTrue)

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
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), gc.Equals, int32(numApplications*totalToAcquirePerApplication))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}

func (s *LimiterSuite) TestGlobalAndApplicationLimit(c *gc.C) {

	const (
		globalLimit                  = 5
		applicationLimit             = 3
		numApplications              = 3
		totalToAcquirePerApplication = 2
	)
	limiter := resource.NewResourceDownloadLimiter(globalLimit, applicationLimit)

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
			limiter.Acquire(uuid)
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
	c.Assert(limitReached, jc.IsTrue)

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
	c.Assert(atomic.LoadInt32(&totalAcquiredCount), gc.Equals, int32(numApplications*totalToAcquirePerApplication))

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting for finish")
	}
}
