// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/trace"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/lease"
)

const (
	defaultMaxSleep = time.Hour
)

var (
	defaultClockStart time.Time
)

func init() {
	// We pick a time with a comfortable h:m:s component but:
	//  (1) past the int32 unix epoch limit;
	//  (2) at a 5ns offset to make sure we're not discarding precision;
	//  (3) in a weird time zone.
	value := "2073-03-03T01:00:00.000000005-08:40"
	var err error
	defaultClockStart, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
}

// offset returns the result of defaultClockStart.Add(d); it exists to make
// expiry tests easier to write.
func offset(d time.Duration) time.Time {
	return defaultClockStart.Add(d)
}

// Fixture allows us to test a *lease.Manager with a usefully-mocked
// clock.Clock and corelease.Store.
type Fixture struct {

	// leases contains the leases the corelease.Store should report when the
	// test starts up.
	leases map[corelease.Key]corelease.Info

	// expectCalls contains the calls that should be made to the corelease.Store
	// in the course of a test. By specifying a callback you can cause the
	// reported leases to change.
	expectCalls []call

	// expectDirty should be set for tests that purposefully abuse the manager
	// to the extent that it returns an error on Wait(); tests that don't set
	// this flag will check that the manager's shutdown error is nil.
	expectDirty bool
}

// RunTest sets up a Manager and a Clock and passes them into the supplied
// test function. The manager will be cleaned up afterwards.
func (fix *Fixture) RunTest(c *tc.C, test func(*lease.Manager, *testclock.Clock)) {
	clock := testclock.NewClock(defaultClockStart)
	store := NewStore(fix.leases, fix.expectCalls, clock)
	manager, err := lease.NewManager(lease.ManagerConfig{
		Clock: clock,
		Store: store,
		SecretaryFinder: FuncSecretaryFinder(func(string) (corelease.Secretary, error) {
			return Secretary{}, nil
		}),
		MaxSleep:             defaultMaxSleep,
		Logger:               loggertesting.WrapCheckLog(c),
		PrometheusRegisterer: noopRegisterer{},
		Tracer:               trace.NoopTracer{},
	})
	c.Assert(err, tc.ErrorIsNil)
	var wg sync.WaitGroup
	testDone := make(chan struct{})
	storeDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Dirty tests will probably have stopped the manager anyway, but no
		// sense leaving them around if things aren't exactly as we expect.
		timeout := time.After(coretesting.LongWait)
		select {
		case <-testDone:
		case <-timeout:
			c.Errorf("test took >10s to complete")
		}
		// Wait for the store to be done in the happy path, but
		// don't wait any longer than 10s total.
		select {
		case <-storeDone:
		case <-time.After(coretesting.LongWait):
			c.Errorf("store took >10s to complete")
		}
		manager.Kill()
		err := manager.Wait()
		if !fix.expectDirty {
			c.Check(err, tc.ErrorIsNil)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		store.Wait(c)
		close(storeDone)
	}()
	waitAlarms(c, clock, 1)
	test(manager, clock)
	close(testDone)
	wg.Wait()
}

func waitAlarms(c *tc.C, clock *testclock.Clock, count int) {
	timeout := time.After(coretesting.LongWait)
	for i := 0; i < count; i++ {
		select {
		case <-clock.Alarms():
		case <-timeout:
			c.Fatalf("timed out waiting for %dth alarm set", i)
		}
	}
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return false
}
