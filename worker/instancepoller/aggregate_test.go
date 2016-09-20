// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

type aggregateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&aggregateSuite{})

type testInstance struct {
	instance.Instance
	id        instance.Id
	addresses []network.Address
	status    string
	err       error
}

var _ instance.Instance = (*testInstance)(nil)

func (t *testInstance) Id() instance.Id {
	return t.id
}

func (t *testInstance) Addresses() ([]network.Address, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.addresses, nil
}

func (t *testInstance) Status() instance.InstanceStatus {
	return instance.InstanceStatus{Status: status.Unknown, Message: t.status}
}

type testInstanceGetter struct {
	sync.RWMutex
	// ids is set when the Instances method is called.
	ids     []instance.Id
	results map[instance.Id]instance.Instance
	err     error
	counter int32
}

func (tig *testInstanceGetter) Instances(ids []instance.Id) (result []instance.Instance, err error) {
	tig.ids = ids
	atomic.AddInt32(&tig.counter, 1)
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		// We don't check 'ok' here, because we want the Instance{nil}
		// response for those
		results[i] = tig.results[id]
	}
	return results, tig.err
}

func (tig *testInstanceGetter) newTestInstance(id instance.Id, status string, addresses []string) *testInstance {
	if tig.results == nil {
		tig.results = make(map[instance.Id]instance.Instance)
	}
	thisInstance := &testInstance{
		id:        id,
		status:    status,
		addresses: network.NewAddresses(addresses...),
	}
	tig.results[thisInstance.Id()] = thisInstance
	return thisInstance
}

// Test that one request gets sent after suitable delay.
func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	// We setup a couple variables here so that we can use them locally without
	// type assertions. Then we use them in the aggregatorConfig.
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Minute
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	// Add a new test instance.
	testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	// Ensure the worker is killed and cleaned up if the test exits early.
	defer workertest.CleanKill(c, aggregator)

	// Create a test in a goroutine and make sure we wait for it to finish.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		info, err := aggregator.instanceInfo("foo")
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.status.Message, gc.DeepEquals, "foobar")
	}()

	// Unwind the test clock
	waitAlarms(c, clock, 1)
	clock.Advance(delay)

	wg.Wait()

	// Ensure we kill the worker before looking at our testInstanceGetter to
	// ensure there's no possibility of a race.
	workertest.CleanKill(c, aggregator)

	ids := testGetter.ids
	c.Assert(ids, gc.DeepEquals, []instance.Id{"foo"})
}

// Test several requests in a short space of time get batched.
func (s *aggregateSuite) TestMultipleResponseHandling(c *gc.C) {
	// We setup a couple variables here so that we can use them locally without
	// type assertions. Then we use them in the aggregatorConfig.
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Minute
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	// Setup multiple instances to batch
	testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.newTestInstance("foo2", "not foobar", []string{"192.168.1.2"})
	testGetter.newTestInstance("foo3", "ok-ish", []string{"192.168.1.3"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	// Ensure the worker is killed and cleaned up if the test exits early.
	defer workertest.CleanKill(c, aggregator)

	// Create a closure for tests we can launch in goroutines.
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string) {
		defer wg.Done()
		info, err := aggregator.instanceInfo(id)
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.status.Message, gc.Equals, expectStatus)
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo2", "not foobar")
	go checkInfo("foo3", "ok-ish")

	// Unwind the testing clock to let our requests through.
	waitAlarms(c, clock, 2)
	clock.Advance(delay)

	// Check we're still alive.
	workertest.CheckAlive(c, aggregator)

	// Wait until the tests pass.
	wg.Wait()

	// Ensure we kill the worker before looking at our testInstanceGetter to
	// ensure there's no possibility of a race.
	workertest.CleanKill(c, aggregator)

	// Ensure we got our list back with the expected contents.
	c.Assert(testGetter.ids, jc.SameContents, []instance.Id{"foo2", "foo3"})

	// Ensure we called instances once and have no errors there.
	c.Assert(testGetter.err, jc.ErrorIsNil)
	c.Assert(testGetter.counter, gc.DeepEquals, int32(1))
}

// Test that advancing delay-time.Nanosecond and then killing causes all
// pending reqs to fail.
func (s *aggregateSuite) TestKillingWorkerKillsPendinReqs(c *gc.C) {
	// Setup local variables.
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Minute
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.newTestInstance("foo2", "not foobar", []string{"192.168.1.2"})
	testGetter.newTestInstance("foo3", "ok-ish", []string{"192.168.1.3"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	defer workertest.CleanKill(c, aggregator)

	// Set up a couple tests we can launch.
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id) {
		defer wg.Done()
		info, err := aggregator.instanceInfo(id)
		c.Check(err.Error(), gc.Equals, "instanceInfo call aborted")
		c.Check(info.status.Message, gc.Equals, "")
	}

	// Launch a couple tests.
	wg.Add(2)
	go checkInfo("foo2")

	// Advance the clock and kill the worker.
	waitAlarms(c, clock, 1)
	clock.Advance(delay - time.Nanosecond)
	aggregator.Kill()

	go checkInfo("foo3")

	// Make sure we're dead.
	workertest.CheckKilled(c, aggregator)
	wg.Wait()

	// Make sure we have no ids, since we're dead.
	c.Assert(len(testGetter.ids), gc.DeepEquals, 0)

	// Ensure we called instances once and have no errors there.
	c.Assert(testGetter.err, jc.ErrorIsNil)
	c.Assert(testGetter.counter, gc.DeepEquals, int32(0))

}

// Test that having sent/advanced/received one batch, you can
// send/advance/receive again and that works, too.
func (s *aggregateSuite) TestMultipleBatches(c *gc.C) {
	// Setup some local variables.
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Second
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.newTestInstance("foo2", "not foobar", []string{"192.168.1.2"})
	testGetter.newTestInstance("foo3", "ok-ish", []string{"192.168.1.3"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	// Ensure the worker is killed and cleaned up if the test exits early.
	defer workertest.CleanKill(c, aggregator)

	// Create a checker we can launch as goroutines
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string) {
		defer wg.Done()
		info, err := aggregator.instanceInfo(id)
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.status.Message, gc.Equals, expectStatus)
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo2", "not foobar")
	go checkInfo("foo3", "ok-ish")

	// Unwind the testing clock to let our requests through.
	waitAlarms(c, clock, 2)
	clock.Advance(delay)

	// Check we're still alive
	workertest.CheckAlive(c, aggregator)

	// Wait until the checkers pass
	// TODO(redir): These could block forever, we should make the effort to be
	// robust here per http://reviews.vapour.ws/r/4885/
	wg.Wait()

	// Ensure we got our list back with the expected length.
	c.Assert(len(testGetter.ids), gc.DeepEquals, 2)

	// And then a second batch
	testGetter.newTestInstance("foo4", "spam", []string{"192.168.1.4"})
	testGetter.newTestInstance("foo5", "eggs", []string{"192.168.1.5"})

	// Launch and wait for this second batch
	wg.Add(2)
	go checkInfo("foo4", "spam")
	go checkInfo("foo5", "eggs")

	for i := 0; i < 2; i++ {
		// Unwind again to let our next batch through.
		<-clock.Alarms()
	}
	// // Advance the clock again.
	clock.Advance(delay)

	// Check we're still alive
	workertest.CheckAlive(c, aggregator)

	// Wait until the checkers pass
	wg.Wait()

	// Shutdown the worker.
	workertest.CleanKill(c, aggregator)

	// Ensure we got our list back with the correct length
	c.Assert(len(testGetter.ids), gc.DeepEquals, 2)

	// Ensure we called instances once and have no errors there.
	c.Assert(testGetter.err, jc.ErrorIsNil)
	c.Assert(testGetter.counter, gc.Equals, int32(2))
}

// Test that things behave as expected when env.Instances errors.
func (s *aggregateSuite) TestInstancesErrors(c *gc.C) {
	// Setup local variables.
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Millisecond
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.newTestInstance("foo", "foobar", []string{"192.168.1.2"})
	testGetter.err = environs.ErrNoInstances
	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	defer workertest.CleanKill(c, aggregator)

	// Launch test in a goroutine and wait for it.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err = aggregator.instanceInfo("foo")
		c.Assert(err, gc.Equals, environs.ErrNoInstances)
	}()

	// Unwind to let our request through.
	waitAlarms(c, clock, 1)
	clock.Advance(delay)

	wg.Wait()

	// Kill the worker so we know there is no race checking the erroringTestGetter.
	workertest.CleanKill(c, aggregator)

	c.Assert(testGetter.err, gc.Equals, environs.ErrNoInstances)
	c.Assert(testGetter.counter, gc.Equals, int32(1))
}

func (s *aggregateSuite) TestPartialInstanceErrors(c *gc.C) {
	testGetter := new(testInstanceGetter)
	clock := jujutesting.NewClock(time.Now())
	delay := time.Second

	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.err = environs.ErrPartialInstances
	testGetter.newTestInstance("foo", "not foobar", []string{"192.168.1.2"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	// Ensure the worker is killed and cleaned up if the test exits early.
	defer workertest.CleanKill(c, aggregator)

	// // Create a checker we can launch as goroutines
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string, expectedError error) {
		defer wg.Done()
		info, err := aggregator.instanceInfo(id)
		if expectedError == nil {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err.Error(), gc.Equals, expectedError.Error())
		}
		c.Check(info.status.Message, gc.Equals, expectStatus)
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo", "not foobar", nil)
	go checkInfo("foo2", "", errors.New("instance foo2 not found"))

	// Unwind the testing clock to let our requests through.
	waitAlarms(c, clock, 2)
	clock.Advance(delay)

	// Check we're still alive.
	workertest.CheckAlive(c, aggregator)

	// Wait until the checkers pass.
	wg.Wait()

	// Now kill the worker so we don't risk a race in the following assertions.
	workertest.CleanKill(c, aggregator)

	// Ensure we got our list back with the correct length.
	c.Assert(len(testGetter.ids), gc.Equals, 2)

	// Ensure we called instances once.
	// TODO(redir): all this stuff is really crying out to be, e.g.
	// testGetter.CheckOneCall(c, "foo", "foo2") per
	// http://reviews.vapour.ws/r/4885/
	c.Assert(testGetter.counter, gc.Equals, int32(1))
}

func waitAlarms(c *gc.C, clock *jujutesting.Clock, count int) {
	timeout := time.After(testing.LongWait)
	for i := 0; i < count; i++ {
		select {
		case <-clock.Alarms():
		case <-timeout:
			c.Fatalf("timed out waiting for %dth alarm set", i)
		}
	}
}
