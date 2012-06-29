package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"time"
)

type ServiceSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *ServiceSuite) TestAddService(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.St.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.St.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.charm.URL().String())
	mysql, err = s.St.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.charm.URL().String())
}

func (s *ServiceSuite) TestRemoveService(c *C) {
	service, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = s.St.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.St.Service("wordpress")
	c.Assert(err, ErrorMatches, `can't get service "wordpress": service with name "wordpress" not found`)

	// Remove of an illegal service, it has already been removed.
	err = s.St.RemoveService(service)
	c.Assert(err, ErrorMatches, `can't remove service "wordpress": can't get all units from service "wordpress": environment state has changed`)
}

func (s *ServiceSuite) TestReadNonExistentService(c *C) {
	_, err := s.St.Service("pressword")
	c.Assert(err, ErrorMatches, `can't get service "pressword": service with name "pressword" not found`)
}

func (s *ServiceSuite) TestAllServices(c *C) {
	services, err := s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	services, err = s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.St.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
	services, err = s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

func (s *ServiceSuite) TestServiceCharm(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	// Check that getting and setting the service charm URL works correctly.
	testcurl, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.charm.URL().String())

	// TODO shouldn't it be an error to set a charm URL that doesn't correspond
	// to a known charm??
	testcurl = charm.MustParseURL("local:myseries/mydummy-1")
	err = wordpress.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *ServiceSuite) TestServiceExposed(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
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
	err = s.St.RemoveService(wordpress)
	c.Assert(err, IsNil)
	err = wordpress.ClearExposed()
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestAddUnit(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
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
	m, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service.
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.St.AddService("logging", subCharm)
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

func (s *ServiceSuite) TestReadUnit(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	mysql, err := s.St.AddService("mysql", s.charm)
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

func (s *ServiceSuite) TestRemoveUnit(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
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

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	wordpress, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)

	// Check that reading a unit after removing the service
	// fails nicely.
	err = s.St.RemoveService(wordpress)
	c.Assert(err, IsNil)
	_, err = s.St.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress/0": can't get service "wordpress": service with name "wordpress" not found`)
}

type ServiceWatcherSuite struct {
	ConnSuite
	service *state.Service
}

var _ = Suite(&ServiceWatcherSuite{})

func (s *ServiceWatcherSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	charm := s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.St.AddService("wordpress", charm)
	c.Assert(err, IsNil)
}

var serviceWatchConfigData = []map[string]interface{}{
	{},
	{"foo": "bar", "baz": "yadda"},
	{"baz": "yadda"},
}

func (s *ServiceWatcherSuite) TestServiceWatchConfig(c *C) {
	config, err := s.service.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)

	configWatcher := s.service.WatchConfig()
	defer func() {
		c.Assert(configWatcher.Stop(), IsNil)
	}()

	// Two change events.
	config.Set("foo", "bar")
	config.Set("baz", "yadda")
	changes, err := config.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []state.ItemChange{{
		Key:      "baz",
		Type:     state.ItemAdded,
		NewValue: "yadda",
	}, {
		Key:      "foo",
		Type:     state.ItemAdded,
		NewValue: "bar",
	}})
	time.Sleep(100 * time.Millisecond)
	config.Delete("foo")
	changes, err = config.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []state.ItemChange{{
		Key:      "foo",
		Type:     state.ItemDeleted,
		OldValue: "bar",
	}})

	for _, want := range serviceWatchConfigData {
		select {
		case got, ok := <-configWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.Map(), DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got := <-configWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceWatcherSuite) TestServiceWatchConfigIllegalData(c *C) {
	configWatcher := s.service.WatchConfig()
	defer func() {
		c.Assert(configWatcher.Stop(), ErrorMatches, "unmarshall error: YAML error: .*")
	}()

	// Receive empty change after service adding.
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{})
	case <-time.After(100 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}

	// Set config to illegal data.
	_, err := s.zkConn.Set("/services/service-0000000000/config", "---", -1)
	c.Assert(err, IsNil)

	select {
	case _, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(100 * time.Millisecond):
	}
}

var serviceExposedTests = []struct {
	test func(s *state.Service) error
	want bool
}{
	{func(s *state.Service) error { return nil }, false},
	{func(s *state.Service) error { return s.SetExposed() }, true},
	{func(s *state.Service) error { return s.ClearExposed() }, false},
	{func(s *state.Service) error { return s.SetExposed() }, true},
}

func (s *ServiceWatcherSuite) TestServiceWatchExposed(c *C) {
	exposedWatcher := s.service.WatchExposed()
	defer func() {
		c.Assert(exposedWatcher.Stop(), IsNil)
	}()

	for i, test := range serviceExposedTests {
		c.Logf("test %d", i)
		err := test.test(s.service)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-exposedWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", test.want)
		}
	}

	select {
	case got := <-exposedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceWatcherSuite) TestServiceWatchExposedContent(c *C) {
	exposedWatcher := s.service.WatchExposed()
	defer func() {
		c.Assert(exposedWatcher.Stop(), IsNil)
	}()

	s.service.SetExposed()
	select {
	case got, ok := <-exposedWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got, Equals, true)
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("didn't get change: %#v", true)
	}

	// Re-set exposed with some data.
	_, err := s.zkConn.Set("/services/service-0000000000/exposed", "some: data", -1)
	c.Assert(err, IsNil)

	select {
	case got := <-exposedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

var serviceUnitTests = []struct {
	testOp string
	idx    int
}{
	{"none", 0},
	{"add", 0},
	{"add", 1},
	{"remove", 0},
}

func (s *ServiceWatcherSuite) TestServiceWatchUnits(c *C) {
	unitsWatcher := s.service.WatchUnits()
	defer func() {
		c.Assert(unitsWatcher.Stop(), IsNil)
	}()
	units := make([]*state.Unit, 2)

	for i, test := range serviceUnitTests {
		c.Logf("test %d", i)
		var want *state.ServiceUnitsChange
		switch test.testOp {
		case "none":
			want = &state.ServiceUnitsChange{}
		case "add":
			var err error
			units[test.idx], err = s.service.AddUnit()
			c.Assert(err, IsNil)
			want = &state.ServiceUnitsChange{[]*state.Unit{units[test.idx]}, nil}
		case "remove":
			err := s.service.RemoveUnit(units[test.idx])
			c.Assert(err, IsNil)
			want = &state.ServiceUnitsChange{nil, []*state.Unit{units[test.idx]}}
			units[test.idx] = nil
		}
		select {
		case got, ok := <-unitsWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got := <-unitsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
