// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(wallyworld) - move to instancepoller_test
package instancepoller

import (
	stderrors "errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&machineSuite{})

type machineSuite struct {
	coretesting.BaseSuite
}

var testAddrs = network.NewAddresses("127.0.0.1")

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
	// Change the poll intervals to be short, so that we know
	// that we've polled (probably) at least a few times.
	s.PatchValue(&ShortPoll, coretesting.ShortWait/10)
	s.PatchValue(&LongPoll, coretesting.ShortWait/10)

	go runMachine(context, m, nil, died)
	time.Sleep(coretesting.ShortWait)

	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
	c.Assert(m.setAddressCount, gc.Equals, 1)
	c.Assert(m.instStatus, gc.Equals, "running")
}

func (s *machineSuite) TestShortPollIntervalWhenNoAddress(c *gc.C) {
	s.PatchValue(&ShortPoll, 1*time.Millisecond)
	s.PatchValue(&LongPoll, coretesting.LongWait)
	count := countPolls(c, nil, "i1234", "running", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

func (s *machineSuite) TestShortPollIntervalWhenNoStatus(c *gc.C) {
	s.PatchValue(&ShortPoll, 1*time.Millisecond)
	s.PatchValue(&LongPoll, coretesting.LongWait)
	count := countPolls(c, testAddrs, "i1234", "", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

func (s *machineSuite) TestShortPollIntervalWhenNotStarted(c *gc.C) {
	s.PatchValue(&ShortPoll, 1*time.Millisecond)
	s.PatchValue(&LongPoll, coretesting.LongWait)
	count := countPolls(c, testAddrs, "i1234", "pending", params.StatusPending)
	c.Assert(count, jc.GreaterThan, 2)
}

func (s *machineSuite) TestShortPollIntervalWhenNotProvisioned(c *gc.C) {
	s.PatchValue(&ShortPoll, 1*time.Millisecond)
	s.PatchValue(&LongPoll, coretesting.LongWait)
	count := countPolls(c, testAddrs, "", "pending", params.StatusPending)
	c.Assert(count, gc.Equals, 0)
}

func (s *machineSuite) TestShortPollIntervalExponent(c *gc.C) {
	s.PatchValue(&ShortPoll, 1*time.Microsecond)
	s.PatchValue(&LongPoll, coretesting.LongWait)
	s.PatchValue(&ShortPollBackoff, 2.0)

	// With an exponent of 2, the maximum number of polls that can
	// occur within the given interval ShortWait is log to the base
	// ShortPollBackoff of ShortWait/ShortPoll, given that sleep will
	// sleep for at least the requested interval.
	maxCount := int(math.Log(float64(coretesting.ShortWait)/float64(ShortPoll))/math.Log(ShortPollBackoff) + 1)
	count := countPolls(c, nil, "i1234", "", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
	c.Assert(count, jc.LessThan, maxCount)
	c.Logf("actual count: %v; max %v", count, maxCount)
}

func (s *machineSuite) TestLongPollIntervalWhenHasAllInstanceInfo(c *gc.C) {
	s.PatchValue(&ShortPoll, coretesting.LongWait)
	s.PatchValue(&LongPoll, 1*time.Millisecond)
	count := countPolls(c, testAddrs, "i1234", "running", params.StatusStarted)
	c.Assert(count, jc.GreaterThan, 2)
}

// countPolls sets up a machine loop with the given
// addresses and status to be returned from getInstanceInfo,
// waits for coretesting.ShortWait, and returns the
// number of times the instance is polled.
func countPolls(c *gc.C, addrs []network.Address, instId, instStatus string, machineStatus params.Status) int {
	count := int32(0)
	getInstanceInfo := func(id instance.Id) (instanceInfo, error) {
		c.Check(string(id), gc.Equals, instId)
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
		tag:        names.NewMachineTag("99"),
		instanceId: instance.Id(instId),
		refresh:    func() error { return nil },
		addresses:  addrs,
		life:       params.Alive,
		status:     machineStatus,
	}
	died := make(chan machine)

	go runMachine(context, m, nil, died)

	time.Sleep(coretesting.ShortWait)
	killMachineLoop(c, m, context.dyingc, died)
	c.Assert(context.killErr, gc.Equals, nil)
	return int(count)
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
		tag:        names.NewMachineTag("99"),
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       params.Alive,
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
	c *gc.C, expectId instance.Id, addrs []network.Address,
	status string, err error) func(id instance.Id) (instanceInfo, error) {

	return func(id instance.Id) (instanceInfo, error) {
		c.Check(id, gc.Equals, expectId)
		return instanceInfo{addrs, status}, err
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
	instStatus      string
	status          params.Status
	refresh         func() error
	setAddressesErr error
	// mu protects the following fields.
	mu              sync.Mutex
	life            params.Life
	addresses       []network.Address
	setAddressCount int
}

func (m *testMachine) Tag() names.MachineTag {
	return m.tag
}

func (m *testMachine) Id() string {
	return m.tag.Id()
}

func (m *testMachine) ProviderAddresses() ([]network.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.addresses, nil
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	if m.instanceId == "" {
		err := &params.Error{
			Code:    params.CodeNotProvisioned,
			Message: fmt.Sprintf("machine %v not provisioned", m.Id()),
		}
		return "", err
	}
	return m.instanceId, m.instanceIdErr
}

// This is stubbed out for testing.
var MachineStatus = func(m *testMachine) (params.StatusResult, error) {
	return params.StatusResult{Status: m.status}, nil
}

func (m *testMachine) Status() (params.StatusResult, error) {
	return MachineStatus(m)
}

func (m *testMachine) IsManual() (bool, error) {
	return strings.HasPrefix(string(m.instanceId), "manual:"), nil
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

func (m *testMachine) SetProviderAddresses(addrs ...network.Address) error {
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

func (m *testMachine) setLife(life params.Life) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.life = life
}
