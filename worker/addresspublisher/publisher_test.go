package addresspublisher

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	 "launchpad.net/juju-core/testing/testbase"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/instance"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&publisherSuite{})

type publisherSuite struct {
}

var testAddrs = []instance.Address{instance.NewAddress("127.0.0.1")}

func (*publisherSuite) TestSetsAddressInitially(c *gc.C) {
	ctxt := &testMachineContext{
		getAddresses: addressesGetter(c, "i1234", testAddrs, nil),
		dyingc: make(chan struct{}),
	}
	m := &testMachine{
		instanceId: "i1234",
		refresh: func() error {return nil},
		life: state.Alive,
	}
	died := make(chan machine)
	publisherc := make(chan machineAddress, 100)
	// Change the poll intervals to be short, so that we know
	// that we've polled (probably) at least a few times while we're
	// waiting for anything to be sent on publisherc.
	defer testbase.PatchValue(&shortPoll, coretesting.ShortWait / 10).Restore()
	defer testbase.PatchValue(&longPoll, coretesting.ShortWait / 10).Restore()
	go runMachine(ctxt, m, nil, died, publisherc)
	select{
	case addr := <-publisherc:
		c.Assert(addr.addresses, gc.DeepEquals, testAddrs)
		c.Assert(addr.machine, gc.Equals, m)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("publisher never published the expected address")
	}
	select{
	case addr := <-publisherc:
		c.Fatalf("address publisher unexpectedly published twice; addr %#v", addr)
	case <-time.After(coretesting.ShortWait):
	}
	close(ctxt.dyingc)
	select{
	case diedm := <-died:
		c.Assert(diedm, gc.Equals, m)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("publisher did not die after dying channel was closed")
	}
	c.Assert(m.addresses, gc.DeepEquals, testAddrs)
}

func addressesGetter(c *gc.C, expectId instance.Id, addrs []instance.Address, err error) func(id instance.Id) ([]instance.Address, error) {
	return func(id instance.Id) ([]instance.Address, error) {
		c.Check(id, gc.Equals, expectId)
		return addrs, err
	}
}

type testMachineContext struct {
	killAllErr error
	getAddresses func(instance.Id) ([]instance.Address, error)
	dyingc chan struct{}
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
	life state.Life
	addresses []instance.Address
	instanceId instance.Id
	jobs []state.MachineJob
	id string
	refresh func() error
}

func (m *testMachine) Addresses() []instance.Address {
	return m.addresses
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	return m.instanceId, nil
}

func (m *testMachine) SetAddresses(addrs []instance.Address) error {
	m.addresses = append(m.addresses[:0], addrs...)
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
	return m.life
}
