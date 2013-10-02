package addressupdater

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

var _ = gc.Suite(&machineSuite{})

type machineSuite struct {
	testbase.LoggingSuite
}

var testAddrs = []instance.Address{instance.NewAddress("127.0.0.1")}

func (*machineSuite) TestSetsAddressInitially(c *gc.C) {
	ctxt := &testMachineContext{
		getAddresses: addressesGetter(c, "i1234", testAddrs, nil),
		dyingc:       make(chan struct{}),
	}
	m := &testMachine{
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       state.Alive,
	}
	died := make(chan machine)
	// Change the poll intervals to be short, so that we know
	// that we've polled (probably) at least a few times.
	defer testbase.PatchValue(&shortPoll, coretesting.ShortWait/10).Restore()
	defer testbase.PatchValue(&longPoll, coretesting.ShortWait/10).Restore()

	go runMachine(ctxt, m, nil, died)
	time.Sleep(coretesting.ShortWait)

	killMachineLoop(c, m, ctxt.dyingc, died)
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
	c.Assert(m.setAddressCount, gc.Equals, 1)
}

func (*machineSuite) TestShortPollIntervalWhenNoAddress(c *gc.C) {
	defer testbase.PatchValue(&shortPoll, 1*time.Millisecond).Restore()
	defer testbase.PatchValue(&longPoll, coretesting.LongWait).Restore()
	testPollInterval(c, nil)
}

func (*machineSuite) TestLongPollIntervalWhenHasAddress(c *gc.C) {
	defer testbase.PatchValue(&shortPoll, coretesting.LongWait).Restore()
	defer testbase.PatchValue(&longPoll, 1*time.Millisecond).Restore()
	testPollInterval(c, testAddrs)
}

// testPollInterval checks that, when the machine and instance addresses
// are set to addrs, it will poll frequently (given the poll intervals set
// up outside this function).
func testPollInterval(c *gc.C, addrs []instance.Address) {
	count := int32(0)
	getAddresses := func(id instance.Id) ([]instance.Address, error) {
		c.Check(id, gc.Equals, instance.Id("i1234"))
		atomic.AddInt32(&count, 1)
		if addrs == nil {
			return nil, fmt.Errorf("no instance addresses available")
		}
		return addrs, nil
	}
	ctxt := &testMachineContext{
		getAddresses: getAddresses,
		dyingc:       make(chan struct{}),
	}
	m := &testMachine{
		instanceId: "i1234",
		refresh:    func() error { return nil },
		addresses:  addrs,
		life:       state.Alive,
	}
	died := make(chan machine)

	go runMachine(ctxt, m, nil, died)

	time.Sleep(coretesting.ShortWait)
	killMachineLoop(c, m, ctxt.dyingc, died)
	c.Assert(count, jc.GreaterThan, 2)
}

func (*machineSuite) TestChangedRefreshes(c *gc.C) {
	ctxt := &testMachineContext{
		getAddresses: addressesGetter(c, "i1234", testAddrs, nil),
		dyingc:       make(chan struct{}),
	}
	refreshc := make(chan struct{})
	m := &testMachine{
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
	go runMachine(ctxt, m, changed, died)
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

// testTerminatingErrors checks that when a testMachine is
// changed with the given mutate function, the machine goroutine
// will die having called its context's killAll function with the
// given error.  The test is cunningly structured so that it in the normal course
// of things it will go through all possible places that can return an error.
func testTerminatingErrors(c *gc.C, mutate func(m *testMachine, err error)) {
	ctxt := &testMachineContext{
		getAddresses: addressesGetter(c, "i1234", testAddrs, nil),
		dyingc:       make(chan struct{}),
	}
	expectErr := errors.New("a very unusual error")
	m := &testMachine{
		instanceId: "i1234",
		refresh:    func() error { return nil },
		life:       state.Alive,
	}
	mutate(m, expectErr)
	died := make(chan machine)
	changed := make(chan struct{}, 1)
	go runMachine(ctxt, m, changed, died)
	changed <- struct{}{}
	select {
	case <-died:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for machine to die")
	}
	c.Assert(ctxt.killAllErr, gc.ErrorMatches, ".*"+expectErr.Error())
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

func addressesGetter(c *gc.C, expectId instance.Id, addrs []instance.Address, err error) func(id instance.Id) ([]instance.Address, error) {
	return func(id instance.Id) ([]instance.Address, error) {
		c.Check(id, gc.Equals, expectId)
		return addrs, err
	}
}

type testMachineContext struct {
	killAllErr   error
	getAddresses func(instance.Id) ([]instance.Address, error)
	dyingc       chan struct{}
}

func (ctxt *testMachineContext) killAll(err error) {
	if err == nil {
		panic("killAll with nil error")
	}
	ctxt.killAllErr = err
}

func (ctxt *testMachineContext) addresses(id instance.Id) ([]instance.Address, error) {
	return ctxt.getAddresses(id)
}

func (ctxt *testMachineContext) dying() <-chan struct{} {
	return ctxt.dyingc
}

type testMachine struct {
	instanceId      instance.Id
	instanceIdErr   error
	jobs            []state.MachineJob
	id              string
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

func (m *testMachine) Jobs() []state.MachineJob {
	return m.jobs
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
