// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	stderrors "errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&machineSuite{})

type machineSuite struct {
	testbase.LoggingSuite
}

var testAddrs = []instance.Address{instance.NewAddress("127.0.0.1")}

func (*machineSuite) TestSetsInstanceInfoInitially(c *gc.C) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       state.Alive,
	}
	died := make(chan machine)
	// Change the poll intervals to be short, so that we know
	// that we've polled (probably) at least a few times.
	defer testbase.PatchValue(&ShortPoll, coretesting.ShortWait/10).Restore()
	defer testbase.PatchValue(&LongPoll, coretesting.ShortWait/10).Restore()

	go runMachine(context, m, nil, died)
	time.Sleep(coretesting.ShortWait)

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killAllErr, gc.Equals, nil)
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
	c.Assert(m.setAddressCount, gc.Equals, 1)
	c.Assert(m.instStatus, gc.Equals, "running")
}

func (*machineSuite) TestShortPollIntervalWhenNoAddress(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, 1*time.Millisecond).Restore()
	defer testbase.PatchValue(&LongPoll, coretesting.LongWait).Restore()
	count := countPolls(c, nil, "running", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

func (*machineSuite) TestShortPollIntervalWhenNoStatus(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, 1*time.Millisecond).Restore()
	defer testbase.PatchValue(&LongPoll, coretesting.LongWait).Restore()
	count := countPolls(c, testAddrs, "", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

func (*machineSuite) TestShortPollIntervalWhenNotStarted(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, 1*time.Millisecond).Restore()
	defer testbase.PatchValue(&LongPoll, coretesting.LongWait).Restore()
	count := countPolls(c, testAddrs, "pending", params.StatusPending)
	c.Assert(count, jc.GreaterThan, 2)
}

func (*machineSuite) TestShortPollIntervalExponent(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, 1*time.Microsecond).Restore()
	defer testbase.PatchValue(&LongPoll, coretesting.LongWait).Restore()
	defer testbase.PatchValue(&ShortPollBackoff, 2.0).Restore()

	// With an exponent of 2, the maximum number of polls that can
	// occur within the given interval ShortWait is log to the base
	// ShortPollBackoff of ShortWait/ShortPoll, given that sleep will
	// sleep for at least the requested interval.
	maxCount := int(math.Log(float64(coretesting.ShortWait)/float64(ShortPoll))/math.Log(ShortPollBackoff) + 1)
	count := countPolls(c, nil, "", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
	c.Assert(count, jc.LessThan, maxCount)
	c.Logf("actual count: %v; max %v", count, maxCount)
}

func (*machineSuite) TestLongPollIntervalWhenHasAllInstanceInfo(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, coretesting.LongWait).Restore()
	defer testbase.PatchValue(&LongPoll, 1*time.Millisecond).Restore()
	count := countPolls(c, testAddrs, "running", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

// countPolls sets up a machine loop with the given
// addresses and status to be returned from getInstanceInfo,
// waits for coretesting.ShortWait, and returns the
// number of times the instance is polled.
func countPolls(c *gc.C, addrs []instance.Address, instStatus string, machineStatus params.Status) int {
	count := int32(0)
	getInstanceInfo := func(id instance.Id) (instanceInfo, error) {
		c.Check(id, gc.Equals, instance.Id("i1234"))
		atomic.AddInt32(&count, 1)
		if addrs == nil {
			return instanceInfo{}, fmt.Errorf("no instance addresses available")
		}
		return instanceInfo{addrs, instStatus}, nil
	}
	context := &testMachineContext{
		getInstanceInfo: getInstanceInfo,
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		refresh:    func() error { return nil },
		addresses:  addrs,
		life:       state.Alive,
		status:     machineStatus,
	}
	died := make(chan machine)

	go runMachine(context, m, nil, died)

	time.Sleep(coretesting.ShortWait)
	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killAllErr, gc.Equals, nil)
	return int(count)
}

func (*machineSuite) TestSinglePollWhenInstancInfoUnimplemented(c *gc.C) {
	defer testbase.PatchValue(&ShortPoll, 1*time.Millisecond).Restore()
	defer testbase.PatchValue(&LongPoll, 1*time.Millisecond).Restore()
	count := int32(0)
	getInstanceInfo := func(id instance.Id) (instanceInfo, error) {
		c.Check(id, gc.Equals, instance.Id("i1234"))
		atomic.AddInt32(&count, 1)
		return instanceInfo{}, errors.NewNotImplementedError("instance address")
	}
	context := &testMachineContext{
		getInstanceInfo: getInstanceInfo,
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       state.Alive,
	}
	died := make(chan machine)

	go runMachine(context, m, nil, died)

	time.Sleep(coretesting.ShortWait)
	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killAllErr, gc.Equals, nil)
	c.Assert(count, gc.Equals, int32(1))
}

func (*machineSuite) TestChangedRefreshes(c *gc.C) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
		dyingc:          make(chan struct{}),
	}
	refreshc := make(chan struct{})
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		refresh: func() error {
			refreshc <- struct{}{}
			return nil
		},
		addresses: testAddrs,
		life:      state.Dead,
	}
	died := make(chan machine)
	changed := make(chan struct{})
	go runMachine(context, m, changed, died)
	select {
	case <-died:
		c.Fatalf("machine died prematurely")
	case <-time.After(coretesting.ShortWait):
	}

	// Notify the machine that it has changed; it should
	// refresh, and publish the fact that it no longer has
	// an address.
	changed <- struct{}{}

	select {
	case <-refreshc:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for refresh")
	}
	select {
	case <-died:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("expected death after life set to dying")
	}
	// The machine addresses should remain the same even
	// after death.
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
}

var terminatingErrorsTests = []struct {
	about  string
	mutate func(m *testMachine, err error)
}{{
	about: "set addresses",
	mutate: func(m *testMachine, err error) {
		m.setAddressesErr = err
	},
}, {
	about: "refresh",
	mutate: func(m *testMachine, err error) {
		m.refresh = func() error {
			return err
		}
	},
}, {
	about: "instance id",
	mutate: func(m *testMachine, err error) {
		m.instanceIdErr = err
	},
}}

func (*machineSuite) TestTerminatingErrors(c *gc.C) {
	for i, test := range terminatingErrorsTests {
		c.Logf("test %d: %s", i, test.about)
		testTerminatingErrors(c, test.mutate)
	}
}

//
// testTerminatingErrors checks that when a testMachine is
// changed with the given mutate function, the machine goroutine
// will die having called its context's killAll function with the
// given error.  The test is cunningly structured so that it in the normal course
// of things it will go through all possible places that can return an error.
func testTerminatingErrors(c *gc.C, mutate func(m *testMachine, err error)) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
		dyingc:          make(chan struct{}),
	}
	expectErr := stderrors.New("a very unusual error")
	m := &testMachine{
		id:         "99",
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       state.Alive,
	}
	mutate(m, expectErr)
	died := make(chan machine)
	changed := make(chan struct{}, 1)
	go runMachine(context, m, changed, died)
	changed <- struct{}{}
	select {
	case <-died:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for machine to die")
	}
	c.Assert(context.killAllErr, gc.ErrorMatches, ".*"+expectErr.Error())
}

func killMachineLoop(c *gc.C, m machine, dying chan struct{}, died <-chan machine) {
	close(dying)
	select {
	case diedm := <-died:
		c.Assert(diedm, gc.Equals, m)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("updater did not die after dying channel was closed")
	}
}

func instanceInfoGetter(
	c *gc.C, expectId instance.Id, addrs []instance.Address,
	status string, err error) func(id instance.Id) (instanceInfo, error) {

	return func(id instance.Id) (instanceInfo, error) {
		c.Check(id, gc.Equals, expectId)
		return instanceInfo{addrs, status}, err
	}
}

type testMachineContext struct {
	killAllErr      error
	getInstanceInfo func(instance.Id) (instanceInfo, error)
	dyingc          chan struct{}
}

func (context *testMachineContext) killAll(err error) {
	if err == nil {
		panic("killAll with nil error")
	}
	context.killAllErr = err
}

func (context *testMachineContext) instanceInfo(id instance.Id) (instanceInfo, error) {
	return context.getInstanceInfo(id)
}

func (context *testMachineContext) dying() <-chan struct{} {
	return context.dyingc
}

type testMachine struct {
	instanceId      instance.Id
	instanceIdErr   error
	id              string
	instStatus      string
	status          params.Status
	refresh         func() error
	setAddressesErr error
	// mu protects the following fields.
	mu              sync.Mutex
	life            state.Life
	addresses       []instance.Address
	setAddressCount int
}

func (m *testMachine) Id() string {
	if m.id == "" {
		panic("Id called but not set")
	}
	return m.id
}

func (m *testMachine) Addresses() []instance.Address {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addresses
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	return m.instanceId, m.instanceIdErr
}

func (m *testMachine) Status() (status params.Status, info string, data params.StatusData, err error) {
	return m.status, "", nil, nil
}

func (m *testMachine) InstanceStatus() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instStatus, nil
}

func (m *testMachine) SetInstanceStatus(status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instStatus = status
	return nil
}

func (m *testMachine) SetAddresses(addrs []instance.Address) error {
	if m.setAddressesErr != nil {
		return m.setAddressesErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addresses = append(m.addresses[:0], addrs...)
	m.setAddressCount++
	return nil
}

func (m *testMachine) String() string {
	return m.id
}

func (m *testMachine) Refresh() error {
	return m.refresh()
}

func (m *testMachine) Life() state.Life {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.life
}

func (m *testMachine) setLife(life state.Life) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.life = life
}
