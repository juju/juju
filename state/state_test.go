package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	stdtesting "testing"
	"time"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	srv := testing.StartZkServer()
	defer srv.Destroy()
	var err error
	state.TestingZkAddr, err = srv.Addr()
	if err != nil {
		t.Fatalf("could not get zk server address")
	}
	TestingT(t)
}

type StateSuite struct {
	zkServer   *zookeeper.Server
	zkTestRoot string
	zkTestPort int
	zkAddr     string
	zkConn     *zookeeper.Conn
	st         *state.State
	ch         charm.Charm
	curl       *charm.URL
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *C) {
	var err error
	s.st, err = state.Initialize(&state.Info{
		Addrs: []string{state.TestingZkAddr},
	})
	c.Assert(err, IsNil)
	s.zkConn = state.ZkConn(s.st)
	s.ch = testing.Charms.Dir("dummy")
	url := fmt.Sprintf("local:series/%s-%d", s.ch.Meta().Name, s.ch.Revision())
	s.curl = charm.MustParseURL(url)
}

func (s *StateSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, "/")
	s.zkConn.Close()
}

func (s *StateSuite) assertMachineCount(c *C, expect int) {
	ms, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestInitialize(c *C) {
	info := &state.Info{
		Addrs: []string{state.TestingZkAddr},
	}
	// Check that initialization of an already-initialized state succeeds.
	st, err := state.Initialize(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	st.Close()

	// Check that Open blocks until Initialize has succeeded.
	testing.ZkRemoveTree(s.zkConn, "/")

	errc := make(chan error)
	go func() {
		st, err := state.Open(info)
		errc <- err
		if st != nil {
			st.Close()
		}
	}()

	// Wait a little while to verify that it's actually blocking.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-errc:
		c.Fatalf("state.Open did not block (returned error %v)", err)
	default:
	}

	st, err = state.Initialize(info)
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	defer st.Close()

	select {
	case err := <-errc:
		c.Assert(err, IsNil)
	case <-time.After(1e9):
		c.Fatalf("state.Open blocked forever")
	}
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms works correctly.
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	children, _, err := s.zkConn.Children("/charms")
	c.Assert(err, IsNil)
	c.Assert(children, DeepEquals, []string{"local_3a_series_2f_dummy-1"})
}

// addDummyCharm adds the 'dummy' charm state to st.
func (s *StateSuite) addDummyCharm(c *C) *state.Charm {
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return dummy
}

func (s *StateSuite) TestCharmAttributes(c *C) {
	// Check that the basic (invariant) fields of the charm
	// are correctly in place.
	s.addDummyCharm(c)

	dummy, err := s.st.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	c.Assert(dummy.BundleSha256(), Equals, "dummy-sha256")
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s *StateSuite) TestNonExistentCharmPriorToInitialization(c *C) {
	// Check that getting a charm before any other charm has been added fails nicely.
	_, err := s.st.Charm(s.curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:series/dummy-1": .*`)
}

func (s *StateSuite) TestGetNonExistentCharm(c *C) {
	// Check that getting a non-existent charm fails nicely.
	s.addDummyCharm(c)

	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:anotherseries/dummy-1": .*`)
}

func (s *StateSuite) TestAddMachine(c *C) {
	machine0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000000", "machine-0000000001"})
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)
	err = s.st.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000001"})

	// Removing a non-existing machine has to fail.
	err = s.st.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "can't remove machine 0: machine not found")
}

func (s *StateSuite) TestMachineInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.st, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", "spaceship/0")
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, "spaceship/0")
}

func (s *StateSuite) TestMachineInstanceIdCorrupt(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.st, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", map[int]int{})
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err.Error(), Equals, "invalid internal machine id type map[interface {}]interface {} for machine 0")
	c.Assert(id, Equals, "")
}

func (s *StateSuite) TestMachineInstanceIdMissing(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, new(state.NoInstanceIdError))
	c.Assert(id, Equals, "")
}

func (s *StateSuite) TestMachineInstanceIdBlank(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	config, err := state.ReadConfigNode(s.st, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	config.Set("provider-machine-id", "")
	_, err = config.Write()
	c.Assert(err, IsNil)

	id, err := machine.InstanceId()
	c.Assert(err, FitsTypeOf, new(state.NoInstanceIdError))
	c.Assert(id, Equals, "")
}

func (s *StateSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	actual, err := state.ReadConfigNode(s.st, fmt.Sprintf("/machines/machine-%010d", machine.Id()))
	c.Assert(err, IsNil)
	c.Assert(actual.Map(), DeepEquals, map[string]interface{}{"provider-machine-id": "umbrella/0"})
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.st.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestReadNonExistentMachine(c *C) {
	_, err := s.st.Machine(0)
	c.Assert(err, ErrorMatches, "machine 0 not found")

	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.st.Machine(1)
	c.Assert(err, ErrorMatches, "machine 1 not found")
}

func (s *StateSuite) TestAllMachines(c *C) {
	s.assertMachineCount(c, 0)
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	s.assertMachineCount(c, 1)
	_, err = s.st.AddMachine()
	c.Assert(err, IsNil)
	s.assertMachineCount(c, 2)
}

func (s *StateSuite) TestMachineSetAgentAlive(c *C) {
	machine0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)

	alive, err := machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := machine0.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *StateSuite) TestMachineWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	machine0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)

	alive, err := machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = machine0.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of machine 0: presence: still not alive after timeout`)

	pinger, err := machine0.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = machine0.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = machine0.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func (s *StateSuite) TestMachineUnits(c *C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	dummy := s.addDummyCharm(c)
	logging := addLoggingCharm(c, s.st)
	s0, err := s.st.AddService("s0", dummy)
	c.Assert(err, IsNil)
	s1, err := s.st.AddService("s1", dummy)
	c.Assert(err, IsNil)
	s2, err := s.st.AddService("s2", dummy)
	c.Assert(err, IsNil)
	s3, err := s.st.AddService("s3", logging)
	c.Assert(err, IsNil)

	units := make([][]*state.Unit, 4)
	for i, svc := range []*state.Service{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = svc.AddUnit()
			c.Assert(err, IsNil)
		}
	}
	// Add the logging units subordinate to the s2 units.
	units[3] = make([]*state.Unit, 3)
	for i := range units[3] {
		units[3][i], err = s3.AddUnitSubordinateTo(units[2][i])
	}

	assignments := []struct {
		machine      *state.Machine
		units        []*state.Unit
		subordinates []*state.Unit
	}{
		{m0, []*state.Unit{units[0][0]}, nil},
		{m1, []*state.Unit{units[0][1], units[1][0], units[1][1], units[2][0]}, []*state.Unit{units[3][0]}},
		{m2, []*state.Unit{units[2][2]}, []*state.Unit{units[3][2]}},
	}

	for _, a := range assignments {
		for _, u := range a.units {
			err := u.AssignToMachine(a.machine)
			c.Assert(err, IsNil)
		}
	}

	for i, a := range assignments {
		c.Logf("test %d", i)
		got, err := a.machine.Units()
		c.Assert(err, IsNil)
		expect := sortedUnitNames(append(a.units, a.subordinates...))
		c.Assert(sortedUnitNames(got), DeepEquals, expect)
	}
}

func sortedUnitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	sort.Strings(names)
	return names
}

func (s *StateSuite) TestAddService(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.curl.String())
	mysql, err = s.st.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.curl.String())
}

func (s *StateSuite) TestRemoveService(c *C) {
	dummy := s.addDummyCharm(c)
	service, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = s.st.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.st.Service("wordpress")
	c.Assert(err, ErrorMatches, `can't get service "wordpress": service with name "wordpress" not found`)

	// Remove of an illegal service, it has already been removed.
	err = s.st.RemoveService(service)
	c.Assert(err, ErrorMatches, `can't remove service "wordpress": can't get all units from service "wordpress": environment state has changed`)
}

func (s *StateSuite) TestReadNonExistentService(c *C) {
	_, err := s.st.Service("pressword")
	c.Assert(err, ErrorMatches, `can't get service "pressword": service with name "pressword" not found`)
}

func (s *StateSuite) TestAllServices(c *C) {
	services, err := s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	dummy := s.addDummyCharm(c)
	_, err = s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

func (s *StateSuite) TestServiceCharm(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that getting and setting the service charm URL works correctly.
	testcurl, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.curl.String())
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = wordpress.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *StateSuite) TestServiceExposed(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that querying for the exposed flag works correctly.
	exposed, err := wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag works correctly.
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	exposed, err = wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, true)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
	exposed, err = wordpress.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag multiple doesn't fail.
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)

	// Check that setting and clearing the exposed flag on removed services also doesn't fail.
	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestAddUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that principal units can be added on their own.
	unitZero, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "wordpress/0")
	principal := unitZero.IsPrincipal()
	c.Assert(principal, Equals, true)
	unitOne, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "wordpress/1")
	principal = unitOne.IsPrincipal()
	c.Assert(principal, Equals, true)

	// Check that principal units cannot be added to principal units.
	_, err = wordpress.AddUnitSubordinateTo(unitZero)
	c.Assert(err, ErrorMatches, `can't add unit of principal service "wordpress" as a subordinate of "wordpress/0"`)

	// Assign the principal unit to a machine.
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service.
	subCh := addLoggingCharm(c, s.st)
	logging, err := s.st.AddService("logging", subCh)
	c.Assert(err, IsNil)

	// Check that subordinate units can be added to principal units
	subZero, err := logging.AddUnitSubordinateTo(unitZero)
	c.Assert(err, IsNil)
	c.Assert(subZero.Name(), Equals, "logging/0")
	principal = subZero.IsPrincipal()
	c.Assert(principal, Equals, false)

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, m.Id())

	// Check that subordinate units must be added to other units.
	_, err = logging.AddUnit()
	c.Assert(err, ErrorMatches, `cannot directly add units to subordinate service "logging"`)

	// Check that subordinate units cannnot be added to subordinate units.
	_, err = logging.AddUnitSubordinateTo(subZero)
	c.Assert(err, ErrorMatches, "a subordinate unit must be added to a principal unit")
}

func (s *StateSuite) TestReadUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving a unit works correctly.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = wordpress.Unit("wordpress")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress" from service "wordpress": "wordpress" is not a valid unit name`)
	unit, err = wordpress.Unit("wordpress/0/0")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress/0/0" from service "wordpress": "wordpress/0/0" is not a valid unit name`)
	unit, err = wordpress.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `can't get unit "pressword/0" from service "wordpress": unit not found`)
	unit, err = wordpress.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `can't get unit "mysql/0" from service "wordpress": unit not found`)

	// Check that retrieving all units works.
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "wordpress/0")
	c.Assert(units[1].Name(), Equals, "wordpress/1")
}

func (s *StateSuite) TestReadUnitWithChangingState(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that reading a unit after removing the service
	// fails nicely.
	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)
	_, err = s.st.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress/0": can't get service "wordpress": service with name "wordpress" not found`)
}

func (s *StateSuite) TestRemoveUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that removing a unit works.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, IsNil)

	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	c.Assert(units[0].Name(), Equals, "wordpress/1")

	// Check that removing a non-existent unit fails nicely.
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, ErrorMatches, `can't unassign unit "wordpress/0" from machine: environment state has changed`)
}

func (s *StateSuite) TestGetSetPublicAddress(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving and setting of a public address works.
	address, err := unit.PublicAddress()
	c.Assert(err, ErrorMatches, "unit has no public address")
	err = unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *StateSuite) TestGetSetPrivateAddress(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving and setting of a private address works.
	address, err := unit.PrivateAddress()
	c.Assert(err, ErrorMatches, "unit has no private address")
	err = unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
}

func (s *StateSuite) TestUnitCharm(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that getting and setting the unit charm URL works correctly.
	testcurl, err := unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.curl.String())
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = unit.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *StateSuite) TestUnassignUnitFromMachineWithoutBeingAssigned(c *C) {
	// When unassigning a machine from a unit, it is possible that
	// the machine has not been previously assigned, or that it
	// was assigned but the state changed beneath us.  In either
	// case, the end state is the intended state, so we simply
	// move forward without any errors here, to avoid having to
	// handle the extra complexity of dealing with the concurrency
	// problems.
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, IsNil)

	// Check that the unit has no machine assigned.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	unit = units[0]
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `can't get machine id of unit "wordpress/0": unit not assigned to machine`)
}

func (s *StateSuite) TestAssignUnitToMachineAgainFails(c *C) {
	// Check that assigning an already assigned unit to
	// a machine fails if it isn't precisely the same
	// machine. 
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	machineOne, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	machineTwo, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to the same machine should return no error.
	err = unit.AssignToMachine(machineOne)
	c.Assert(err, IsNil)

	// Assigning the unit to a different machine should fail.
	err = unit.AssignToMachine(machineTwo)
	c.Assert(err, ErrorMatches, `can't assign unit "wordpress/0" to machine 1: unit already assigned to machine 0`)

	machineId, err := unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(machineId, Equals, 0)
}

func (s *StateSuite) TestUnassignUnitFromMachineWithChangingState(c *C) {
	// Check that unassigning while the state changes fails nicely.
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Remove the unit for the tests.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	unit = units[0]
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `can't unassign unit "wordpress/0" from machine: environment state has changed`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `can't get machine id of unit "wordpress/0": environment state has changed`)

	err = s.st.RemoveService(wordpress)
	c.Assert(err, IsNil)

	err = unit.UnassignFromMachine()
	c.Assert(err, ErrorMatches, `can't unassign unit "wordpress/0" from machine: environment state has changed`)
	_, err = unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `can't get machine id of unit "wordpress/0": environment state has changed`)
}

func (s *StateSuite) TestAssignUnitToUnusedMachine(c *C) {
	// Create root machine that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that a unit can be assigned to an unused machine.
	dummy := s.addDummyCharm(c)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	wordpressMachine, err := wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, IsNil)

	c.Assert(wordpressMachine.Id(), Equals, mysqlMachine.Id())
}

func (s *StateSuite) TestAssignUnitToUnusedMachineWithChangingService(c *C) {
	// Create root machine that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check for a 'state changed' error if a service is manipulated
	// during reuse.
	dummy := s.addDummyCharm(c)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	err = s.st.RemoveService(wordpressService)
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `can't assign unit "wordpress/0" to unused machine: environment state has changed`)
}

func (s *StateSuite) TestAssignUnitToUnusedMachineWithChangingUnit(c *C) {
	// Create root machine that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check for a 'state changed' error if a unit is manipulated
	// during reuse.
	dummy := s.addDummyCharm(c)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)
	err = s.st.RemoveService(mysqlService)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)
	err = wordpressService.RemoveUnit(wordpressUnit)
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `can't assign unit "wordpress/0" to unused machine: environment state has changed`)
}

func (s *StateSuite) TestAssignUnitToUnusedMachineOnlyZero(c *C) {
	// Create root machine that shouldn't be useds.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that the unit can't be assigned to machine zero.
	dummy := s.addDummyCharm(c)
	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *StateSuite) TestAssignUnitToUnusedMachineNoneAvailable(c *C) {
	// Create machine 0, that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that assigning without unused machine fails.
	dummy := s.addDummyCharm(c)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)

	wordpressService, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	wordpressUnit, err := wordpressService.AddUnit()
	c.Assert(err, IsNil)

	_, err = wordpressUnit.AssignToUnusedMachine()
	c.Assert(err, ErrorMatches, `all machines in use`)
}

func (s *StateSuite) TestAssignSubsidiariesToMachine(c *C) {
	// Create machine 0, that shouldn't be used.
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	// Check that assigning a principal unit assigns its subordinates too.
	dummy := s.addDummyCharm(c)
	logging := addLoggingCharm(c, s.st)
	mysqlService, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	logService1, err := s.st.AddService("logging1", logging)
	c.Assert(err, IsNil)
	logService2, err := s.st.AddService("logging2", logging)
	c.Assert(err, IsNil)
	mysqlUnit, err := mysqlService.AddUnit()
	c.Assert(err, IsNil)
	log1Unit, err := logService1.AddUnitSubordinateTo(mysqlUnit)
	c.Assert(err, IsNil)
	log2Unit, err := logService2.AddUnitSubordinateTo(mysqlUnit)
	c.Assert(err, IsNil)

	mysqlMachine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = mysqlUnit.AssignToMachine(mysqlMachine)
	c.Assert(err, IsNil)

	id, err := log1Unit.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Check(id, Equals, mysqlMachine.Id())
	id, err = log2Unit.AssignedMachineId()
	c.Check(id, Equals, mysqlMachine.Id())

	// Check that unassigning the principal unassigns the
	// subordinates too.
	err = mysqlUnit.UnassignFromMachine()
	c.Assert(err, IsNil)
	_, err = log1Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `can't get machine id of unit "logging1/0": unit not assigned to machine`)
	_, err = log2Unit.AssignedMachineId()
	c.Assert(err, ErrorMatches, `can't get machine id of unit "logging2/0": unit not assigned to machine`)
}

func (s *StateSuite) TestAssignUnit(c *C) {
	_, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	dummy := s.addDummyCharm(c)
	serv, err := s.st.AddService("minecraft", dummy)
	c.Assert(err, IsNil)
	unit0, err := serv.AddUnit()
	c.Assert(err, IsNil)

	// Check nonsensical policy
	fail := func() { s.st.AssignUnit(unit0, state.AssignmentPolicy("random")) }
	c.Assert(fail, PanicMatches, `unknown unit assignment policy: "random"`)
	_, err = unit0.AssignedMachineId()
	c.Assert(err, NotNil)
	s.assertMachineCount(c, 1)

	// Check local placement
	err = s.st.AssignUnit(unit0, state.AssignLocal)
	c.Assert(err, IsNil)
	mid, err := unit0.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 0)
	s.assertMachineCount(c, 1)

	// Check unassigned placement with no unused machines
	unit1, err := serv.AddUnit()
	c.Assert(err, IsNil)
	err = s.st.AssignUnit(unit1, state.AssignUnused)
	c.Assert(err, IsNil)
	mid, err = unit1.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 1)
	s.assertMachineCount(c, 2)

	// Check unassigned placement on an unused machine
	_, err = s.st.AddMachine()
	unit2, err := serv.AddUnit()
	c.Assert(err, IsNil)
	err = s.st.AssignUnit(unit2, state.AssignUnused)
	c.Assert(err, IsNil)
	mid, err = unit2.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, 2)
	s.assertMachineCount(c, 3)

	// Check cannot assign subordinates to machines
	subCh := addLoggingCharm(c, s.st)
	logging, err := s.st.AddService("logging", subCh)
	c.Assert(err, IsNil)
	unit3, err := logging.AddUnitSubordinateTo(unit2)
	c.Assert(err, IsNil)
	err = s.st.AssignUnit(unit3, state.AssignUnused)
	c.Assert(err, ErrorMatches, `subordinate unit "logging/0" cannot be assigned directly to a machine`)
}

// addLoggingCharm adds a "logging" (subordinate) charm
// to the state.
func addLoggingCharm(c *C, st *state.State) *state.Charm {
	bundle := testing.Charms.Bundle(c.MkDir(), "logging")
	curl := charm.MustParseURL("cs:series/logging-99")
	bundleURL, err := url.Parse("http://subordinate.url")
	c.Assert(err, IsNil)
	ch, err := st.AddCharm(bundle, curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return ch
}

func (s *StateSuite) TestGetSetClearUnitUpgrade(c *C) {
	// Check that setting and clearing an upgrade flag on a unit works.
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Defaults to false and false.
	needsUpgrade, err := unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be set.
	err = unit.SetNeedsUpgrade(false)
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, false})

	// Can be set multiple times.
	err = unit.SetNeedsUpgrade(false)
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, false})

	// Can be cleared.
	err = unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be cleared multiple times
	err = unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be set forced.
	err = unit.SetNeedsUpgrade(true)
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, true})

	// Can be set forced multiple times.
	err = unit.SetNeedsUpgrade(true)
	c.Assert(err, IsNil)
	needsUpgrade, err = unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, true})

	// Can't be set multipe with different force flag.
	err = unit.SetNeedsUpgrade(false)
	c.Assert(err, ErrorMatches, `can't inform unit "wordpress/0" about upgrade: upgrade already enabled`)
}

func (s *StateSuite) TestGetSetClearResolved(c *C) {
	// Check that setting and clearing the resolved setting on a unit works.
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	setting, err := unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `can't set resolved mode for unit "wordpress/0": flag already set`)
	retry, err := unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(retry, Equals, state.ResolvedNoHooks)

	err = unit.ClearResolved()
	c.Assert(err, IsNil)
	setting, err = unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)
	err = unit.ClearResolved()
	c.Assert(err, IsNil)

	err = unit.SetResolved(state.ResolvedMode(999))
	c.Assert(err, ErrorMatches, `can't set resolved mode for unit "wordpress/0": invalid error resolution mode: 999`)
}

func (s *StateSuite) TestGetOpenPorts(c *C) {
	// Check that changes to the open ports of units work porperly.
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Verify no open ports before activity.
	open, err := unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, HasLen, 0)

	// Now open and close port.
	err = unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
	})

	err = unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
	})

	err = unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})

	err = unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})
}

func (s *StateSuite) TestUnitSetAgentAlive(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	alive, err := unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *StateSuite) TestUnitWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)

	alive, err := unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = unit.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of unit "wordpress/0": presence: still not alive after timeout`)

	pinger, err := unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = unit.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func assertNoRelations(c *C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func (s *StateSuite) TestAddRelationErrors(c *C) {
	dummy := s.addDummyCharm(c)
	req, err := s.st.AddService("req", dummy)
	c.Assert(err, IsNil)
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}

	// Check we can't add a relation until both services exist.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.st.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": service with name "pro" not found`)
	assertNoRelations(c, req)
	pro, err := s.st.AddService("pro", dummy)
	c.Assert(err, IsNil)

	// Check that interfaces have to match.
	proep2 := state.RelationEndpoint{"pro", "other", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.st.AddRelation(proep2, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": endpoints do not relate`)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)

	// Check a variety of surprising endpoint combinations.
	err = s.st.AddRelation(reqep)
	c.Assert(err, ErrorMatches, `can't add relation "req:bar": single endpoint must be a peer relation`)
	assertNoRelations(c, req)

	peer, err := s.st.AddService("peer", dummy)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	err = s.st.AddRelation(peerep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz req:bar": endpoints do not relate`)
	assertNoRelations(c, peer)
	assertNoRelations(c, req)

	err = s.st.AddRelation(peerep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz peer:baz": endpoints do not relate`)
	assertNoRelations(c, peer)

	err = s.st.AddRelation()
	c.Assert(err, ErrorMatches, `can't add relation "": can't relate 0 endpoints`)
	err = s.st.AddRelation(proep, reqep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar peer:baz": can't relate 3 endpoints`)
}

func assertOneRelation(c *C, srv *state.Service, relId int, endpoints ...state.RelationEndpoint) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	rel := rels[0]
	c.Assert(rel.Id(), Equals, relId)
	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, IsNil)
	c.Assert(ep, DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.RelationEndpoint{expectEp})
}

func (s *StateSuite) TestProviderRequirerRelation(c *C) {
	dummy := s.addDummyCharm(c)
	req, err := s.st.AddService("req", dummy)
	c.Assert(err, IsNil)
	pro, err := s.st.AddService("pro", dummy)
	c.Assert(err, IsNil)
	assertNoRelations(c, req)
	assertNoRelations(c, pro)

	// Add a relation, and check we can only do so once.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}
	err = s.st.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	err = s.st.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": relation already exists`)
	assertOneRelation(c, pro, 0, proep, reqep)
	assertOneRelation(c, req, 0, reqep, proep)

	// Remove the relation, and check it can't be removed again.
	err = s.st.RemoveRelation(proep, reqep)
	c.Assert(err, IsNil)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)
	err = s.st.RemoveRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't remove relation "pro:foo req:bar": relation doesn't exist`)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	reqep.RelationScope = state.ScopeContainer
	err = s.st.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	// After adding relation, make proep container-scoped as well, for
	// simplicity of testing.
	proep.RelationScope = state.ScopeContainer
	assertOneRelation(c, pro, 1, proep, reqep)
	assertOneRelation(c, req, 1, reqep, proep)
}

func (s *StateSuite) TestPeerRelation(c *C) {
	dummy := s.addDummyCharm(c)
	peer, err := s.st.AddService("peer", dummy)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	assertNoRelations(c, peer)

	// Add a relation, and check we can only do so once.
	err = s.st.AddRelation(peerep)
	c.Assert(err, IsNil)
	err = s.st.AddRelation(peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz": relation already exists`)
	assertOneRelation(c, peer, 0, peerep)

	// Remove the relation, and check it can't be removed again.
	err = s.st.RemoveRelation(peerep)
	c.Assert(err, IsNil)
	assertNoRelations(c, peer)
	err = s.st.RemoveRelation(peerep)
	c.Assert(err, ErrorMatches, `can't remove relation "peer:baz": relation doesn't exist`)
}

func (s *StateSuite) TestEnvironConfig(c *C) {
	path, err := s.zkConn.Create("/environment", "type: dummy\nname: foo\n", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "/environment")

	env, err := s.st.EnvironConfig()
	env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{"type": "dummy", "name": "foo"})
}
