package firewaller_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/firewaller"
	stdtesting "testing"
	"sort"
	"time"
)

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type FirewallerSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	op <-chan dummy.Operation
}

// invalidateEnvironment alters the environment configuration
// so the ConfigNode returned from the watcher will not pass
// validation.
func (s *FirewallerSuite) invalidateEnvironment() error {
	env, err := s.State.EnvironConfig()
	if err != nil {
		return err
	}
	env.Set("name", 1)
	_, err = env.Write()
	return err
}

// fixEnvironment undoes the work of invalidateEnvironment.
func (s *FirewallerSuite) fixEnvironment() error {
	env, err := s.State.EnvironConfig()
	if err != nil {
		return err
	}
	env.Set("name", "testing")
	_, err = env.Write()
	return err
}

var _ = Suite(&FirewallerSuite{})

func (s *FirewallerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)

	op := make(chan dummy.Operation, 500)
	dummy.Listen(op)
	s.op = op

	env, err := environs.NewEnviron(map[string]interface{}{
		"type":      "dummy",
		"zookeeper": true,
		"name":      "testing",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)

	// Sanity check
	info, err := env.StateInfo()
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
	fw, err := firewaller.NewFirewaller(s.StateInfo(c))
	c.Assert(err, IsNil)
	c.Assert(fw.Stop(), IsNil)
}

func (s *FirewallerSuite) TestAddRemoveMachine(c *C) {
	fw, err := firewaller.NewFirewaller(s.StateInfo(c))
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

func (s *FirewallerSuite) TestEnvironmentChange(c *C) {
	fw, err := firewaller.NewFirewaller(s.StateInfo(c))
	c.Assert(err, IsNil)
	defer c.Assert(fw.Stop(), IsNil)
	err = s.invalidateEnvironment()
	c.Assert(err, IsNil)
	err = s.fixEnvironment()
	c.Assert(err, IsNil)
}

func (s *FirewallerSuite) TestFirewallerStopOnStateClose(c *C) {
	fw, err := firewaller.NewFirewaller(s.StateInfo(c))
	c.Assert(err, IsNil)
	fw.CloseState()
	c.Check(fw.Wait(), ErrorMatches, ".* zookeeper is closing")
	c.Assert(fw.Stop(), ErrorMatches, ".* zookeeper is closing")
}
