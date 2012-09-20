package state_test

import (
	"fmt"
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

	// TODO: SetCharm must validate the change (version, relations, etc)
	wp := s.AddTestingCharm(c, "wordpress")

	testWhenDying(c, s.service, notAliveErr, notAliveErr, func() error {
		err := s.service.SetCharm(wp, true)
		if err == nil {
			ch, force, err1 := s.service.Charm()
			c.Assert(err1, IsNil)
			c.Assert(ch.URL(), DeepEquals, wp.URL())
			c.Assert(force, Equals, true)
		}
		return err
	})
}

func (s *ServiceSuite) TestServiceRefesh(c *C) {
	s1, err := s.State.Service(s.service.Name())
	c.Assert(err, IsNil)

	err = s.service.SetCharm(s.charm, true)
	c.Assert(err, IsNil)

	testch, force, err := s1.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, false)
	c.Assert(testch.URL(), DeepEquals, s.charm.URL())

	err = s1.Refresh()
	c.Assert(err, IsNil)
	testch, force, err = s1.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, true)
	c.Assert(testch.URL(), DeepEquals, s.charm.URL())
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

	// Check that setting and clearing the exposed flag repeatedly does not fail.
	err = s.service.SetExposed()
	c.Assert(err, IsNil)
	err = s.service.SetExposed()
	c.Assert(err, IsNil)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)

	testWhenDying(c, s.service, notAliveErr, notAliveErr,
		func() error {
			return s.service.SetExposed()
		},
		func() error {
			return s.service.ClearExposed()
		})
}

func (s *ServiceSuite) TestAddSubordinateUnitWhenNotAlive(c *C) {
	loggingCharm := s.AddTestingCharm(c, "logging")
	loggingService, err := s.State.AddService("logging", loggingCharm)
	c.Assert(err, IsNil)
	principalService, err := s.State.AddService("svc", s.charm)
	c.Assert(err, IsNil)
	principalUnit, err := principalService.AddUnit()
	c.Assert(err, IsNil)

	const errPat = ".*: service or principal unit are not alive"
	// Test that AddUnitSubordinateTo fails when the principal unit is
	// not alive.
	testWhenDying(c, principalUnit, errPat, errPat, func() error {
		_, err := loggingService.AddUnitSubordinateTo(principalUnit)
		return err
	})

	// Test that AddUnitSubordinateTo fails when the service is
	// not alive.
	principalUnit, err = principalService.AddUnit()
	c.Assert(err, IsNil)
	testWhenDying(c, loggingService, errPat, errPat, func() error {
		_, err := loggingService.AddUnitSubordinateTo(principalUnit)
		return err
	})
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
	c.Assert(err, ErrorMatches, `cannot add unit to service "mysql" as a subordinate of "mysql/0": service is not a subordinate`)

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
	c.Assert(err, ErrorMatches, `cannot add unit to service "logging": unit is a subordinate`)

	// Check that subordinate units cannnot be added to subordinate units.
	_, err = logging.AddUnitSubordinateTo(subZero)
	c.Assert(err, ErrorMatches, `cannot add unit to service "logging" as a subordinate of "logging/0": unit is not a principal`)

	const errPat = ".*: service is not alive"
	testWhenDying(c, s.service, errPat, errPat, func() error {
		_, err = s.service.AddUnit()
		return err
	})
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
	c.Assert(err, ErrorMatches, `cannot get unit "mysql" from service "mysql":.*`)
	unit, err = s.service.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql/0/0" from service "mysql": .*`)
	unit, err = s.service.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `cannot get unit "pressword/0" from service "mysql": .*`)

	// Add another service to check units are not misattributed.
	mysql, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	// BUG(aram): use error strings from state.
	unit, err = s.service.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `cannot get unit "wordpress/0" from service "mysql": .*`)

	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(sortedUnitNames(units), DeepEquals, []string{"mysql/0", "mysql/1"})
}

func (s *ServiceSuite) TestReadUnitWhenDying(c *C) {
	// Test that we can read units from the service whatever
	// their life state.
	_, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	checkAllUnits := func() error {
		// Check that retrieving all units works.
		_, err := s.service.AllUnits()
		return err
	}
	checkUnit := func() error {
		_, err := s.service.Unit("mysql/0")
		return err
	}

	testWhenDying(c, s.service, noErr, noErr, checkAllUnits, checkUnit)

	unit0, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	testWhenDying(c, unit0, noErr, noErr, checkAllUnits, checkUnit)
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
	c.Assert(err, ErrorMatches, `cannot remove unit "mysql/0": unit is not dead`)
	err = unit.Die()
	c.Assert(err, IsNil)
	err = s.service.RemoveUnit(unit)
	c.Assert(err, IsNil)

	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	c.Assert(units[0].Name(), Equals, "mysql/1")

	err = s.service.RemoveUnit(unit)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.State.RemoveService(s.service)
	c.Assert(err, ErrorMatches, `cannot remove service "mysql": service is not dead`)
	err = s.service.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql/0": not found`)
}

func (s *ServiceSuite) TestServiceConfig(c *C) {
	env, err := s.service.Config()
	c.Assert(err, IsNil)
	err = env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{})

	env.Update(map[string]interface{}{"spam": "eggs", "eggs": "spam"})
	env.Update(map[string]interface{}{"spam": "spam", "chaos": "emeralds"})
	_, err = env.Write()
	c.Assert(err, IsNil)

	env, err = s.service.Config()
	c.Assert(err, IsNil)
	err = env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{"spam": "spam", "eggs": "spam", "chaos": "emeralds"})
}

var serviceUnitsWatchTests = []struct {
	test    func(*C, *state.State, *state.Service)
	added   []string
	removed []string
}{
	{
		test:  func(_ *C, _ *state.State, _ *state.Service) {},
		added: []string{},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
		},
		added: []string{"mysql/0"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
		},
		added: []string{"mysql/1"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		},
		added: []string{"mysql/2", "mysql/3"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			unit3, err := service.Unit("mysql/3")
			c.Assert(err, IsNil)
			err = unit3.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit3)
			c.Assert(err, IsNil)
		},
		removed: []string{"mysql/3"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			unit0, err := service.Unit("mysql/0")
			c.Assert(err, IsNil)
			err = unit0.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit0)
			c.Assert(err, IsNil)
			unit2, err := service.Unit("mysql/2")
			c.Assert(err, IsNil)
			err = unit2.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit2)
			c.Assert(err, IsNil)
		},
		removed: []string{"mysql/0", "mysql/2"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			unit1, err := service.Unit("mysql/1")
			c.Assert(err, IsNil)
			err = unit1.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit1)
			c.Assert(err, IsNil)
		},
		added:   []string{"mysql/4"},
		removed: []string{"mysql/1"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			units := [20]*state.Unit{}
			var err error
			for i := 0; i < len(units); i++ {
				units[i], err = service.AddUnit()
				c.Assert(err, IsNil)
			}
			for i := 10; i < len(units); i++ {
				err = units[i].Die()
				c.Assert(err, IsNil)
				err = service.RemoveUnit(units[i])
				c.Assert(err, IsNil)
			}
		},
		added: []string{"mysql/10", "mysql/11", "mysql/12", "mysql/13", "mysql/14", "mysql/5", "mysql/6", "mysql/7", "mysql/8", "mysql/9"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			unit9, err := service.Unit("mysql/9")
			c.Assert(err, IsNil)
			err = unit9.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit9)
			c.Assert(err, IsNil)
		},
		added:   []string{"mysql/25"},
		removed: []string{"mysql/9"},
	},
	{
		test: func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
			ch, _, err := service.Charm()
			c.Assert(err, IsNil)
			svc, err := s.AddService("bacon", ch)
			c.Assert(err, IsNil)
			_, err = svc.AddUnit()
			c.Assert(err, IsNil)
			_, err = svc.AddUnit()
			c.Assert(err, IsNil)
			unit14, err := service.Unit("mysql/14")
			c.Assert(err, IsNil)
			err = unit14.Die()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit14)
			c.Assert(err, IsNil)
		},
		added:   []string{"mysql/26", "mysql/27"},
		removed: []string{"mysql/14"},
	},
}

func (s *ServiceSuite) TestWatchUnits(c *C) {
	unitWatcher := s.service.WatchUnits()
	defer func() {
		c.Assert(unitWatcher.Stop(), IsNil)
	}()
	for i, test := range serviceUnitsWatchTests {
		c.Logf("test %d", i)
		test.test(c, s.State, s.service)
		s.State.StartSync()
		got := &state.ServiceUnitsChange{}
		for {
			select {
			case new, ok := <-unitWatcher.Changes():
				c.Assert(ok, Equals, true)
				addUnitChanges(got, new)
				if moreUnitsRequired(got, test.added, test.removed) {
					continue
				}
				assertSameUnits(c, got, test.added, test.removed)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("did not get change, want: added: %#v, removed: %#v, got: %#v", test.added, test.removed, got)
			}
			break
		}
	}
	select {
	case got := <-unitWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func moreUnitsRequired(got *state.ServiceUnitsChange, added, removed []string) bool {
	return len(got.Added)+len(got.Removed) < len(added)+len(removed)
}

func addUnitChanges(changes *state.ServiceUnitsChange, more *state.ServiceUnitsChange) {
	changes.Added = append(changes.Added, more.Added...)
	changes.Removed = append(changes.Removed, more.Removed...)
}

type unitSlice []*state.Unit

func (m unitSlice) Len() int           { return len(m) }
func (m unitSlice) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m unitSlice) Less(i, j int) bool { return m[i].Name() < m[j].Name() }

func assertSameUnits(c *C, change *state.ServiceUnitsChange, added, removed []string) {
	c.Assert(change, NotNil)
	if len(added) == 0 {
		added = nil
	}
	if len(removed) == 0 {
		removed = nil
	}
	sort.Sort(unitSlice(change.Added))
	sort.Sort(unitSlice(change.Removed))
	var got []string
	for _, g := range change.Added {
		got = append(got, g.Name())
	}
	c.Assert(got, DeepEquals, added)
	got = nil
	for _, g := range change.Removed {
		got = append(got, g.Name())
	}
	c.Assert(got, DeepEquals, removed)
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
	relationsWatcher := s.service.WatchRelations()
	defer func() {
		c.Assert(relationsWatcher.Stop(), IsNil)
	}()

	s.State.StartSync()
	// Check initial event, and lack of followup.
	assertChange := func(adds, removes []int) {
		select {
		case change := <-relationsWatcher.Changes():
			assertRelationIds(c, change.Added, adds)
			assertRelationIds(c, change.Removed, removes)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("expected change, got nothing")
		}
	}
	assertChange(nil, nil)
	assertNoChange := func() {
		select {
		case change := <-relationsWatcher.Changes():
			c.Fatalf("expected nothing, got %#v", change)
		case <-time.After(100 * time.Millisecond):
		}
	}
	assertNoChange()

	// Add a couple of services, check no changes.
	_, err := s.State.AddService("wp1", s.charm)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("wp2", s.charm)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertNoChange()

	// Add a relation; check change.
	mysqlep := state.RelationEndpoint{"mysql", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	wp1ep := state.RelationEndpoint{"wp1", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(mysqlep, wp1ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{0}, nil)
	assertNoChange()

	// Add another relation; check change.
	wp2ep := state.RelationEndpoint{"wp2", "ifce", "baz", state.RoleRequirer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(mysqlep, wp2ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{1}, nil)
	assertNoChange()

	s.State.StartSync()
	// Remove a relation; check change.
	err = rel.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange(nil, []int{0})

	// Stop watcher; check change chan is closed.
	err = relationsWatcher.Stop()
	c.Assert(err, IsNil)
	assertClosed := func() {
		select {
		case _, ok := <-relationsWatcher.Changes():
			c.Assert(ok, Equals, false)
		default:
			c.Fatalf("Changes not closed")
		}
	}
	assertClosed()

	// Add a new relation; start a new watcher; check initial event.
	_, err = s.State.AddRelation(mysqlep, wp1ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	relationsWatcher = s.service.WatchRelations()
	s.State.StartSync()
	assertChange([]int{1, 2}, nil)
	assertNoChange()

	// Stop new watcher; check change chan is closed.
	err = relationsWatcher.Stop()
	c.Assert(err, IsNil)
	assertClosed()
}

func (s *ServiceSuite) TestWatchRelationsMultipleEvents(c *C) {
	relationsWatcher := s.service.WatchRelations()
	defer func() {
		c.Assert(relationsWatcher.Stop(), IsNil)
	}()
	s.State.StartSync()
	want := &state.RelationsChange{}
	select {
	case got, ok := <-relationsWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got, DeepEquals, want)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("didn't get change: %#v", want)
	}
	relations := make([]*state.Relation, 5)
	endpoints := make([]state.RelationEndpoint, 5)
	var err error
	mysqlep := state.RelationEndpoint{"mysql", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}

	for i := 0; i < 5; i++ {
		_, err := s.State.AddService("wp"+fmt.Sprint(i), s.charm)
		c.Assert(err, IsNil)
		endpoints[i] = state.RelationEndpoint{"wp" + fmt.Sprint(i), "ifce", "spam" + fmt.Sprint(i), state.RoleRequirer, charm.ScopeGlobal}
		relations[i], err = s.State.AddRelation(mysqlep, endpoints[i])
		c.Assert(err, IsNil)
	}
	err = relations[4].Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveRelation(relations[4])
	c.Assert(err, IsNil)
	relations[4] = nil
	want = &state.RelationsChange{Added: relations[:4]}
	s.State.StartSync()
	got := &state.RelationsChange{}
	for {
		select {
		case new, ok := <-relationsWatcher.Changes():
			c.Assert(ok, Equals, true)
			addRelationChanges(got, new)
			if moreRelationsRequired(got, want) {
				continue
			}
			c.Assert(got, DeepEquals, want)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
		break
	}

	for i := 0; i < 4; i++ {
		err = relations[i].Die()
		c.Assert(err, IsNil)
	}
	want.Removed = relations[:4]
	for i := 0; i < 4; i++ {
		err = s.State.RemoveRelation(relations[i])
		c.Assert(err, IsNil)
	}
	_, err = s.State.AddService("wp", s.charm)
	ep := state.RelationEndpoint{"wp", "ifce", "spam", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(mysqlep, ep)
	c.Assert(err, IsNil)
	want.Added = []*state.Relation{rel}

	s.State.StartSync()
	got = &state.RelationsChange{}
	for {
		select {
		case new, ok := <-relationsWatcher.Changes():
			c.Assert(ok, Equals, true)
			addRelationChanges(got, new)
			if moreRelationsRequired(got, want) {
				continue
			}
			c.Assert(got, DeepEquals, want)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
		break
	}

	select {
	case got := <-relationsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func moreRelationsRequired(got, want *state.RelationsChange) bool {
	return len(got.Added)+len(got.Removed) < len(want.Added)+len(want.Removed)
}

func addRelationChanges(changes *state.RelationsChange, more *state.RelationsChange) {
	changes.Added = append(changes.Added, more.Added...)
	changes.Removed = append(changes.Removed, more.Removed...)
}
