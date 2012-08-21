package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"sort"
	"time"
)

type ServiceSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestServiceCharm(c *C) {
	ch, force, err := s.service.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)

	// TODO: changing the charm like this is not especially sane in itself.
	wp := s.AddTestingCharm(c, "wordpress")
	err = s.service.SetCharm(wp, true)
	c.Assert(err, IsNil)
	ch, force, err = s.service.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, wp.URL())
	c.Assert(force, Equals, true)
}

func (s *ServiceSuite) TestServiceExposed(c *C) {
	// Check that querying for the exposed flag works correctly.
	exposed, err := s.service.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag works correctly.
	err = s.service.SetExposed()
	c.Assert(err, IsNil)
	exposed, err = s.service.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, true)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)
	exposed, err = s.service.IsExposed()
	c.Assert(err, IsNil)
	c.Assert(exposed, Equals, false)

	// Check that setting and clearing the exposed flag multiple doesn't fail.
	err = s.service.SetExposed()
	c.Assert(err, IsNil)
	err = s.service.SetExposed()
	c.Assert(err, IsNil)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestAddUnit(c *C) {
	// Check that principal units can be added on their own.
	unitZero, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "mysql/0")
	principal := unitZero.IsPrincipal()
	c.Assert(principal, Equals, true)
	unitOne, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "mysql/1")
	principal = unitOne.IsPrincipal()
	c.Assert(principal, Equals, true)

	// Check that principal units cannot be added to principal units.
	_, err = s.service.AddUnitSubordinateTo(unitZero)
	c.Assert(err, ErrorMatches, `cannot add unit of principal service "mysql" as a subordinate of "mysql/0"`)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service.
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", subCharm)
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
	_, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	// Check that retrieving a unit works correctly.
	unit, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.service.Unit("mysql")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql" from service "mysql": "mysql" is not a valid unit name`)
	unit, err = s.service.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql/0/0" from service "mysql": "mysql/0/0" is not a valid unit name`)
	unit, err = s.service.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `cannot get unit "pressword/0" from service "mysql": unit not found`)

	// Add another service to check units are not misattributed.
	mysql, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	unit, err = s.service.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `cannot get unit "wordpress/0" from service "mysql": unit not found`)

	// Check that retrieving all units works.
	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "mysql/0")
	c.Assert(units[1].Name(), Equals, "mysql/1")
}

func (s *ServiceSuite) TestRemoveUnit(c *C) {
	_, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.service.AddUnit()
	c.Assert(err, IsNil)

	// Check that removing a unit works.
	unit, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	err = s.service.RemoveUnit(unit)
	c.Assert(err, IsNil)

	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	c.Assert(units[0].Name(), Equals, "mysql/1")

	// Check that removing a non-existent unit fails nicely.
	// TODO improve error message.
	err = s.service.RemoveUnit(unit)
	c.Assert(err, ErrorMatches, `cannot unassign unit "mysql/0" from machine: environment state has changed`)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.State.RemoveService(s.service)
	c.Assert(err, IsNil)
	_, err = s.State.Unit("mysql/0")
	// TODO BUG https://bugs.launchpad.net/juju-core/+bug/1020322
	c.Assert(err, ErrorMatches, `cannot get unit "mysql/0": cannot get service "mysql": service with name "mysql" not found`)
}

var serviceWatchConfigData = []map[string]interface{}{
	{},
	{"foo": "bar", "baz": "yadda"},
	{"baz": "yadda"},
}

func (s *ServiceSuite) TestWatchConfig(c *C) {
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
			c.Fatalf("did not get change: %#v", want)
		}
	}

	select {
	case got := <-configWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceSuite) TestWatchConfigIllegalData(c *C) {
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

func (s *ServiceSuite) TestWatchCharm(c *C) {
	// Check initial event.
	w := s.service.WatchCharm()
	assertChange := func(curl *charm.URL, force bool) {
		select {
		case ch, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(ch.Charm.URL(), DeepEquals, curl)
			c.Assert(ch.Force, Equals, force)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("expected change, got none")
		}
	}
	assertChange(s.charm.URL(), false)

	// Check resetting the same charm has no effect.
	err := s.service.SetCharm(s.charm, false)
	c.Assert(err, IsNil)
	assertNoChange := func() {
		select {
		case ch, ok := <-w.Changes():
			c.Fatalf("got unexpected change: %#v, %t", ch, ok)
		case <-time.After(200 * time.Millisecond):
		}
	}
	assertNoChange()

	// Set the force flag.
	err = s.service.SetCharm(s.charm, true)
	c.Assert(err, IsNil)
	assertChange(s.charm.URL(), true)

	// Set a different charm.
	alt := s.AddTestingCharm(c, "mysql")
	err = s.service.SetCharm(alt, true)
	c.Assert(err, IsNil)
	assertChange(alt.URL(), true)

	// Reset the original charm.
	err = s.service.SetCharm(s.charm, false)
	c.Assert(err, IsNil)
	assertChange(s.charm.URL(), false)

	// Stop the watcher.
	err = w.Stop()
	c.Assert(err, IsNil)
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, false)
	default:
		c.Fatalf("channel not closed")
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

func (s *ServiceSuite) TestWatchExposed(c *C) {
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
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-exposedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceSuite) TestWatchExposedContent(c *C) {
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
		c.Fatalf("did not get change: %#v", true)
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

func (s *ServiceSuite) TestWatchUnits(c *C) {
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
			c.Fatalf("did not get change: %#v", want)
		}
	}

	select {
	case got := <-unitsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func assertRelationIds(c *C, rels []*state.Relation, ids []int) {
	c.Assert(rels, HasLen, len(ids))
	relids := []int{}
	for _, rel := range rels {
		relids = append(relids, rel.Id())
	}
	sort.Ints(ids)
	sort.Ints(relids)
	for i, id := range ids {
		c.Assert(relids[i], Equals, id)
	}
}

func (s *ServiceSuite) TestWatchRelations(c *C) {
	w := s.service.WatchRelations()

	// Check initial event, and lack of followup.
	assertChange := func(adds, removes []int) {
		select {
		case change := <-w.Changes():
			assertRelationIds(c, change.Added, adds)
			assertRelationIds(c, change.Removed, removes)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("expected change, got nothing")
		}
	}
	assertChange(nil, nil)
	assertNoChange := func() {
		select {
		case change := <-w.Changes():
			c.Fatalf("expected nothing, got %#v", change)
		case <-time.After(200 * time.Millisecond):
		}
	}
	assertNoChange()

	// Add a couple of services, check no changes.
	_, err := s.State.AddService("wp1", s.charm)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("wp2", s.charm)
	c.Assert(err, IsNil)
	assertNoChange()

	// Add a relation; check change.
	mysqlep := state.RelationEndpoint{"mysql", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	wp1ep := state.RelationEndpoint{"wp1", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(mysqlep, wp1ep)
	c.Assert(err, IsNil)
	assertChange([]int{0}, nil)
	assertNoChange()

	// Add another relation; check change.
	wp2ep := state.RelationEndpoint{"wp2", "ifce", "baz", state.RoleRequirer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(mysqlep, wp2ep)
	c.Assert(err, IsNil)
	assertChange([]int{1}, nil)
	assertNoChange()

	// Remove one of the relations; check change.
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	assertChange(nil, []int{0})
	assertNoChange()

	// Stop watcher; check change chan is closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed := func() {
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, false)
		default:
			c.Fatalf("Changes not closed")
		}
	}
	assertClosed()

	// Add a new relation; start a new watcher; check initial event.
	rel, err = s.State.AddRelation(mysqlep, wp1ep)
	c.Assert(err, IsNil)
	w = s.service.WatchRelations()
	assertChange([]int{1, 2}, nil)
	assertNoChange()

	// Stop new watcher; check change chan is closed.
	err = w.Stop()
	c.Assert(err, IsNil)
	assertClosed()
}
