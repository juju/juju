// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

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
	return instance.InstanceStatus{Status: status.StatusUnknown, Message: t.status}
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
	clock := testing.NewClock(time.Now())
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
		info, err := aggregator.instanceInfo("foo")
		c.Check(err, jc.ErrorIsNil)
		c.Assert(info.status.Message, gc.DeepEquals, "foobar")
		wg.Done()
	}()

	// Unwind the test clock
	<-clock.Alarms()
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
	clock := testing.NewClock(time.Now())
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
		info, err := aggregator.instanceInfo(id)
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.status.Message, gc.Equals, expectStatus)
		wg.Done()
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo2", "not foobar")
	go checkInfo("foo3", "ok-ish")

	for i := 0; i < 2; i++ {
		// Unwind the testing clock to let our requests through.
		<-clock.Alarms()
	}
	clock.Advance(delay)

	// Check we're still alive.
	workertest.CheckAlive(c, aggregator)

	// Wait until the tests pass.
	wg.Wait()

	// Ensure we kill the worker before looking at our testInstanceGetter to
	// ensure there's no possibility of a race.
	workertest.CleanKill(c, aggregator)

	// Ensure we got our list back with the expected length.
	c.Assert(len(testGetter.ids), gc.DeepEquals, 2)

	// Ensure we called instances once and have no errors there.
	c.Assert(testGetter.err, jc.ErrorIsNil)
	c.Assert(testGetter.counter, gc.DeepEquals, int32(1))
}

// Test that advancing delay-time.Nanosecond and then killing causes all
// pending reqs to fail.
func (s *aggregateSuite) TestKillingWorkerKillsPendinReqs(c *gc.C) {
	// Setup local variables.
	testGetter := new(testInstanceGetter)
	clock := testing.NewClock(time.Now())
	delay := time.Nanosecond
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
	// Advance the clock and kill the worker.
	clock.Advance(delay)
	aggregator.Kill()

	// Set up a couple tests we can launch.
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id) {
		info, err := aggregator.instanceInfo(id)
		c.Check(err.Error(), gc.Equals, "instanceInfo call aborted")
		c.Check(info.status.Message, gc.Equals, "")
		wg.Done()
	}

	// Launch a couple tests.
	wg.Add(2)
	go checkInfo("foo2")
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
	clock := testing.NewClock(time.Now())
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
		info, err := aggregator.instanceInfo(id)
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.status.Message, gc.Equals, expectStatus)
		wg.Done()
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo2", "not foobar")
	go checkInfo("foo3", "ok-ish")

	for i := 0; i < 2; i++ {
		// Unwind the testing clock to let our requests through.
		<-clock.Alarms()
	}
	clock.Advance(delay)

	// Check we're still alive
	workertest.CheckAlive(c, aggregator)

	// Wait until the checkers pass
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

// 5. (and, in all above cases, checking that you've made the ListInstances calls you expect, once you've killed the worker) (done)

// Test that things behave as expected when env.Instances errors.
type erroringTestGetter struct {
	testInstanceGetter
}

func (tig *erroringTestGetter) Instances(ids []instance.Id) (instances []instance.Instance, err error) {
	atomic.AddInt32(&tig.counter, 1)
	tig.err = environs.ErrNoInstances
	results := make([]instance.Instance, 0)
	return results, environs.ErrNoInstances
}

func (s *aggregateSuite) TestInstancesErrors(c *gc.C) {
	// Setup local variables.
	testGetter := new(erroringTestGetter)
	clock := testing.NewClock(time.Now())
	delay := time.Millisecond
	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.newTestInstance("foo", "foobar", []string{"192.168.1.2"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	defer workertest.CleanKill(c, aggregator)

	// Launch test in a goroutine and wait for it.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		_, err = aggregator.instanceInfo("foo")
		c.Assert(err, gc.Equals, environs.ErrNoInstances)
		wg.Done()
	}()

	// Unwind to let our request through.
	<-clock.Alarms()
	clock.Advance(delay)

	wg.Wait()

	// Kill the worker so we know there is no race checking the erroringTestGetter.
	workertest.CleanKill(c, aggregator)

	c.Assert(testGetter.err, gc.Equals, environs.ErrNoInstances)
	c.Assert(testGetter.counter, gc.Equals, int32(1))
}

type partialErroringTestGetter struct {
	testInstanceGetter
}

func (tig *partialErroringTestGetter) Instances(ids []instance.Id) (instances []instance.Instance, err error) {
	tig.ids = ids
	atomic.AddInt32(&tig.counter, 1)
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		// We don't check 'ok' here, because we want the Instance{nil}
		// response for those
		if id == "foo" {
			results[i] = tig.results[id]
		}
	}
	// We want to keep foo but not foo2, and we want to return ErrPartialInstances.
	delete(tig.results, "foo2")
	tig.err = environs.ErrPartialInstances

	return results, tig.err
}

func (s *aggregateSuite) TestPartialInstanceErrors(c *gc.C) {
	testGetter := new(partialErroringTestGetter)
	clock := testing.NewClock(time.Now())
	delay := time.Second

	cfg := aggregatorConfig{
		Clock:   clock,
		Delay:   delay,
		Environ: testGetter,
	}

	testGetter.newTestInstance("foo", "not foobar", []string{"192.168.1.2"})
	testGetter.newTestInstance("foo2", "foobar", []string{"192.168.1.3"})

	aggregator, err := newAggregator(cfg)
	c.Check(err, jc.ErrorIsNil)

	// Ensure the worker is killed and cleaned up if the test exits early.
	defer workertest.CleanKill(c, aggregator)

	// // Create a checker we can launch as goroutines
	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string, expectedError error) {
		info, err := aggregator.instanceInfo(id)
		if expectedError == nil {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err.Error(), gc.Equals, expectedError.Error())
		}
		c.Check(info.status.Message, gc.Equals, expectStatus)
		wg.Done()
	}

	// Launch and wait for these
	wg.Add(2)
	go checkInfo("foo", "not foobar", nil)
	go checkInfo("foo2", "", errors.New("instance foo2 not found"))

	// Unwind the testing clock to let our requests through.
	for i := 0; i < 2; i++ {
		<-clock.Alarms()
	}
	clock.Advance(delay)

	// Check we're still alive.
	workertest.CheckAlive(c, aggregator)

	// Wait until the checkers pass.
	wg.Wait()

	// Now kill the worker so we don't have a race in the following assertions.
	workertest.CleanKill(c, aggregator)

	// Ensure we got our list back with the correct length.
	c.Assert(len(testGetter.ids), gc.Equals, 2)

	// Ensure we called instances once and have an error there.
	c.Assert(testGetter.err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(testGetter.counter, gc.Equals, int32(1))
}
