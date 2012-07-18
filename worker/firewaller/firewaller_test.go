package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/firewaller"
	"sort"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type FirewallerSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	environ environs.Environ
	op      <-chan dummy.Operation
	charm   *state.Charm
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	var err error
	s.environ, err = environs.NewEnviron(map[string]interface{}{
		"type":      "dummy",
		"zookeeper": true,
		"name":      "testing",
	})
	c.Assert(err, IsNil)
	err = s.environ.Bootstrap(false)
	c.Assert(err, IsNil)

	// Sanity check
	info, err := s.environ.StateInfo()
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, s.StateInfo(c))

	s.StateSuite.SetUpTest(c)
}

func (s *FirewallerSuite) TearDownTest(c *C) {
	dummy.Reset()
	s.StateSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *FirewallerSuite) TestStartStop(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ)
	c.Assert(err, IsNil)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestAddRemoveMachine(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ)
	c.Assert(err, IsNil)

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m3, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)

	addedMachines := []int{m1.Id(), m2.Id(), m3.Id()}
	allMachines := fw.AllMachines()
	sort.Ints(addedMachines)
	sort.Ints(allMachines)
	c.Assert(addedMachines, DeepEquals, allMachines)

	err = s.State.RemoveMachine(m2.Id())
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)

	addedMachines = []int{m1.Id(), m3.Id()}
	allMachines = fw.AllMachines()
	sort.Ints(addedMachines)
	sort.Ints(allMachines)
	c.Assert(addedMachines, DeepEquals, allMachines)

	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestAssignUnassignUnit(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ)
	c.Assert(err, IsNil)

	m1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	s.charm = s.AddTestingCharm(c, "dummy")
	s1, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	u1, err := s1.AddUnit()
	c.Assert(err, IsNil)
	err = u1.AssignToMachine(m1)
	c.Assert(err, IsNil)
	u2, err := s1.AddUnit()
	c.Assert(err, IsNil)
	err = u2.AssignToMachine(m2)
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)

	addedUnits := []string{u1.Name(), u2.Name()}
	allUnits := fw.AllUnits()
	sort.Strings(addedUnits)
	sort.Strings(allUnits)
	c.Assert(allUnits, DeepEquals, addedUnits)

	err = u1.UnassignFromMachine()
	c.Assert(err, IsNil)
	time.Sleep(100 * time.Millisecond)

	addedUnits = []string{u2.Name()}
	allUnits = fw.AllUnits()
	sort.Strings(addedUnits)
	sort.Strings(allUnits)
	c.Assert(allUnits, DeepEquals, addedUnits)

	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestFirewallerStopOnStateClose(c *C) {
	fw, err := firewaller.NewFirewaller(s.environ)
	c.Assert(err, IsNil)
	fw.CloseState()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}
