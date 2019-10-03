// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&machineSuite{})

type machineSuite struct {
	coretesting.BaseSuite
}

var testAddrs = network.ProviderAddresses{
	network.NewProviderAddress("127.0.0.1"),
	{
		MachineAddress: network.MachineAddress{
			Value: "10.6.6.6",
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		},
		SpaceName:       "test-space",
		ProviderSpaceID: "1",
	},
}

func (s *machineSuite) TestSetsInstanceInfoInitially(c *gc.C) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       params.Alive,
	}
	died := make(chan machine)

	clk := newTestClock()
	go runMachine(context, m, nil, died, clk)
	c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)
	c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
	c.Assert(m.setAddressCount, gc.Equals, 1)
	c.Assert(m.instStatusInfo, gc.Equals, "running")
}

func (s *machineSuite) TestSetsInstanceInfoDeadMachineInitially(c *gc.C) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "deleting", nil),
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       params.Dead,
	}
	died := make(chan machine)

	clk := newTestClock()
	go runMachine(context, m, nil, died, clk)
	c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)
	c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
	c.Assert(m.setAddressCount, gc.Equals, 0)
	c.Assert(m.instStatusInfo, gc.Equals, "deleting")
}

func (s *machineSuite) TestShortPollIntervalWhenNoAddress(c *gc.C) {
	s.testShortPoll(c, nil, "i1234", "running", status.Started)
}

func (s *machineSuite) TestShortPollIntervalWhenNoStatus(c *gc.C) {
	s.testShortPoll(c, testAddrs, "i1234", "", "")
}

func (s *machineSuite) TestShortPollIntervalWhenNotStarted(c *gc.C) {
	s.testShortPoll(c, testAddrs, "i1234", "pending", status.Pending)
}

func (s *machineSuite) testShortPoll(
	c *gc.C, addrs network.ProviderAddresses,
	instId, instStatus string,
	machineStatus status.Status,
) {
	clk := newTestClock()
	testRunMachine(c, addrs, instId, instStatus, machineStatus, clk, func() {
		c.Assert(clk.WaitAdvance(
			time.Duration(float64(ShortPoll)*ShortPollBackoff), coretesting.ShortWait, 1),
			jc.ErrorIsNil,
		)
	})
	clk.CheckCall(c, 0, "After", time.Duration(float64(ShortPoll)*ShortPollBackoff))
	clk.CheckCall(c, 1, "After", time.Duration(float64(ShortPoll)*ShortPollBackoff*ShortPollBackoff))
}

func (s *machineSuite) TestNoPollWhenNotProvisioned(c *gc.C) {
	polled := make(chan struct{}, 1)
	getInstanceInfo := func(id instance.Id) (instanceInfo, error) {
		select {
		case polled <- struct{}{}:
		default:
		}
		return instanceInfo{testAddrs, instance.Status{Status: status.Unknown, Message: "pending"}}, nil
	}
	context := &testMachineContext{
		getInstanceInfo: getInstanceInfo,
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: instance.Id(""),
		refresh:    func() error { return nil },
		addresses:  testAddrs,
		life:       params.Alive,
		status:     "pending",
	}
	died := make(chan machine)

	clk := testclock.NewClock(time.Time{})
	changed := make(chan struct{})
	go runMachine(context, m, changed, died, clk)

	expectPoll := func() {
		c.Assert(clk.WaitAdvance(ShortPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)
	}

	expectPoll()
	expectPoll()
	select {
	case <-polled:
		c.Fatalf("unexpected instance poll")
	case <-time.After(coretesting.ShortWait):
	}

	m.setInstanceId("inst-ance")
	expectPoll()
	select {
	case <-polled:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("expected instance poll")
	}

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
}

func (s *machineSuite) TestShortPollBackoffLimit(c *gc.C) {
	pollDurations := []time.Duration{
		2 * time.Second, // ShortPoll
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		64 * time.Second,
		128 * time.Second,
		256 * time.Second,
		512 * time.Second,
		900 * time.Second, // limit is 15 minutes (LongPoll)
	}

	clk := newTestClock()
	testRunMachine(c, nil, "i1234", "", status.Started, clk, func() {
		for _, d := range pollDurations {
			c.Assert(clk.WaitAdvance(d, coretesting.ShortWait, 1), jc.ErrorIsNil)
		}
	})
	for i, d := range pollDurations {
		clk.CheckCall(c, i, "After", d)
	}
}

func (s *machineSuite) TestLongPollIntervalWhenHasAllInstanceInfo(c *gc.C) {
	clk := newTestClock()
	testRunMachine(c, testAddrs, "i1234", "running", status.Started, clk, func() {
		c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)
	})
	clk.CheckCall(c, 0, "After", LongPoll)
}

func testRunMachine(
	c *gc.C,
	addrs network.ProviderAddresses,
	instId, instStatus string,
	machineStatus status.Status,
	clock clock.Clock,
	test func(),
) {
	getInstanceInfo := func(id instance.Id) (instanceInfo, error) {
		c.Check(string(id), gc.Equals, instId)
		if addrs == nil {
			return instanceInfo{}, fmt.Errorf("no instance addresses available")
		}
		return instanceInfo{addrs, instance.Status{Status: status.Unknown, Message: instStatus}}, nil
	}
	context := &testMachineContext{
		getInstanceInfo: getInstanceInfo,
		dyingc:          make(chan struct{}),
	}
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: instance.Id(instId),
		refresh:    func() error { return nil },
		addresses:  addrs,
		life:       params.Alive,
		status:     machineStatus,
	}
	died := make(chan machine)

	go runMachine(context, m, nil, died, clock)
	test()

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
}

func (*machineSuite) TestChangedRefreshes(c *gc.C) {
	context := &testMachineContext{
		getInstanceInfo: instanceInfoGetter(c, "i1234", testAddrs, "running", nil),
		dyingc:          make(chan struct{}),
	}
	refreshc := make(chan struct{})
	m := &testMachine{
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		refresh: func() error {
			refreshc <- struct{}{}
			return nil
		},
		addresses: testAddrs,
		life:      params.Dead,
	}
	died := make(chan machine)
	changed := make(chan struct{})
	clk := newTestClock()
	go runMachine(context, m, changed, died, clk)

	c.Assert(clk.WaitAdvance(LongPoll, coretesting.ShortWait, 1), jc.ErrorIsNil)
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
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       params.Alive,
	}
	mutate(m, expectErr)
	died := make(chan machine)
	changed := make(chan struct{}, 1)
	go runMachine(context, m, changed, died, newTestClock())
	changed <- struct{}{}
	select {
	case <-died:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for machine to die")
	}
	c.Assert(context.killErr, gc.ErrorMatches, ".*"+expectErr.Error())
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
	c *gc.C, expectId instance.Id, addrs network.ProviderAddresses,
	instanceStatus string, err error) func(id instance.Id) (instanceInfo, error) {

	return func(id instance.Id) (instanceInfo, error) {
		c.Check(id, gc.Equals, expectId)
		return instanceInfo{addrs, instance.Status{Status: status.Unknown, Message: instanceStatus}}, err
	}
}

type testMachineContext struct {
	killErr         error
	getInstanceInfo func(instance.Id) (instanceInfo, error)
	dyingc          chan struct{}
}

func (context *testMachineContext) kill(err error) {
	if err == nil {
		panic("kill with nil error")
	}
	context.killErr = err
}

func (context *testMachineContext) instanceInfo(id instance.Id) (instanceInfo, error) {
	return context.getInstanceInfo(id)
}

func (context *testMachineContext) dying() <-chan struct{} {
	return context.dyingc
}

func (context *testMachineContext) errDying() error {
	return nil
}

type testMachine struct {
	instanceId      instance.Id
	instanceIdErr   error
	tag             names.MachineTag
	instStatus      status.Status
	instStatusInfo  string
	status          status.Status
	refresh         func() error
	setAddressesErr error
	// mu protects the following fields.
	mu              sync.Mutex
	life            params.Life
	addresses       network.ProviderAddresses
	setAddressCount int
}

func (m *testMachine) Tag() names.MachineTag {
	return m.tag
}

func (m *testMachine) Id() string {
	return m.tag.Id()
}

func (m *testMachine) ProviderAddresses() (network.ProviderAddresses, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.addresses, nil
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.instanceId == "" {
		err := &params.Error{
			Code:    params.CodeNotProvisioned,
			Message: fmt.Sprintf("machine %v not provisioned", m.Id()),
		}
		return "", err
	}
	return m.instanceId, m.instanceIdErr
}

func (m *testMachine) setInstanceId(id instance.Id) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instanceId = id
}

func (m *testMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId()
	return instId, "", err
}

// This is stubbed out for testing.
var MachineStatus = func(m *testMachine) (params.StatusResult, error) {
	return params.StatusResult{Status: m.status.String()}, nil
}

func (m *testMachine) Status() (params.StatusResult, error) {
	return MachineStatus(m)
}

func (m *testMachine) IsManual() (bool, error) {
	return strings.HasPrefix(string(m.instanceId), "manual:"), nil
}

func (m *testMachine) InstanceStatus() (params.StatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return params.StatusResult{Status: m.instStatus.String()}, nil
}

func (m *testMachine) SetInstanceStatus(machineStatus status.Status, info string, data map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instStatus = machineStatus
	m.instStatusInfo = info
	return nil
}

func (m *testMachine) SetProviderAddresses(addrs ...network.ProviderAddress) error {
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
	return m.tag.Id()
}

func (m *testMachine) Refresh() error {
	return m.refresh()
}

func (m *testMachine) Life() params.Life {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.life
}

type testClock struct {
	gitjujutesting.Stub
	*testclock.Clock
}

func newTestClock() *testClock {
	clk := testclock.NewClock(time.Time{})
	return &testClock{Clock: clk}
}

func (t *testClock) After(d time.Duration) <-chan time.Time {
	t.MethodCall(t, "After", d)
	return t.Clock.After(d)
}
