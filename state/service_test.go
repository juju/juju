package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
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
	url, force := s.service.CharmURL()
	c.Assert(url, DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)

	// TODO: SetCharm must validate the change (version, relations, etc)
	wp := s.AddTestingCharm(c, "wordpress")
	err = s.service.SetCharm(wp, true)
	ch, force, err1 := s.service.Charm()
	c.Assert(err1, IsNil)
	c.Assert(ch.URL(), DeepEquals, wp.URL())
	c.Assert(force, Equals, true)
	url, force = s.service.CharmURL()
	c.Assert(url, DeepEquals, wp.URL())
	c.Assert(force, Equals, true)

	testWhenDying(c, s.service, notAliveErr, notAliveErr, func() error {
		return s.service.SetCharm(wp, true)
	})
}

func (s *ServiceSuite) TestEndpoints(c *C) {
	// Check state for charm with no explicit relations.
	eps, err := s.service.Endpoints("")
	c.Assert(err, IsNil)
	jujuInfo := state.Endpoint{
		ServiceName:   "mysql",
		Interface:     "juju-info",
		RelationName:  "juju-info",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	}
	c.Assert(eps, DeepEquals, []state.Endpoint{jujuInfo})
	checkCommonNames := func() {
		eps, err = s.service.Endpoints("juju-info")
		c.Assert(err, IsNil)
		c.Assert(eps, DeepEquals, []state.Endpoint{jujuInfo})

		_, err = s.service.Endpoints("voodoo-economy")
		c.Assert(err, ErrorMatches, `service "mysql" has no "voodoo-economy" relation`)
	}
	checkCommonNames()

	// Set a new charm, with a few relations.
	path := testing.Charms.ClonedDirPath(c.MkDir(), "series", "dummy")
	metaPath := filepath.Join(path, "metadata.yaml")
	f, err := os.OpenFile(metaPath, os.O_WRONLY|os.O_APPEND, 0644)
	c.Assert(err, IsNil)
	_, err = f.Write([]byte(`
provides:
  db: mysql
  db-admin: mysql
requires:
  foo:
    interface: bar
    scope: container
peers:
  pressure: pressure
`))
	f.Close()
	c.Assert(err, IsNil)
	ch, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(
		// Fake everything; just use a different URL.
		ch, s.charm.URL().WithRevision(99), s.charm.BundleURL(), s.charm.BundleSha256(),
	)
	c.Assert(err, IsNil)
	err = s.service.SetCharm(sch, false)
	c.Assert(err, IsNil)

	// Check endpoint filtering.
	checkCommonNames()
	pressure := state.Endpoint{
		ServiceName:   "mysql",
		Interface:     "pressure",
		RelationName:  "pressure",
		RelationRole:  state.RolePeer,
		RelationScope: charm.ScopeGlobal,
	}
	eps, err = s.service.Endpoints("pressure")
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.Endpoint{pressure})

	// Check the full list of endpoints.
	eps, err = s.service.Endpoints("")
	c.Assert(err, IsNil)
	c.Assert(eps, HasLen, 5)
	actual := map[string]state.Endpoint{}
	for _, ep := range eps {
		actual[ep.RelationName] = ep
	}
	c.Assert(actual, DeepEquals, map[string]state.Endpoint{
		"juju-info": jujuInfo,
		"pressure":  pressure,
		"db": state.Endpoint{
			ServiceName:   "mysql",
			Interface:     "mysql",
			RelationName:  "db",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeGlobal,
		},
		"db-admin": state.Endpoint{
			ServiceName:   "mysql",
			Interface:     "mysql",
			RelationName:  "db-admin",
			RelationRole:  state.RoleProvider,
			RelationScope: charm.ScopeGlobal,
		},
		"foo": state.Endpoint{
			ServiceName:   "mysql",
			Interface:     "bar",
			RelationName:  "foo",
			RelationRole:  state.RoleRequirer,
			RelationScope: charm.ScopeContainer,
		},
	})
}

func (s *ServiceSuite) TestServiceRefresh(c *C) {
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

	err = s.service.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)
	err = s.service.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *ServiceSuite) TestServiceExposed(c *C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.service.IsExposed(), Equals, false)

	// Check that setting and clearing the exposed flag works correctly.
	err := s.service.SetExposed()
	c.Assert(err, IsNil)
	c.Assert(s.service.IsExposed(), Equals, true)
	err = s.service.ClearExposed()
	c.Assert(err, IsNil)
	c.Assert(s.service.IsExposed(), Equals, false)

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
	removeAllUnits(c, loggingService)
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
	m, err := s.State.AddMachine(state.MachinerWorker)
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

	removeAllUnits(c, s.service)
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

	// Check that retrieving a unit from the service works correctly.
	unit, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a unit from state works correctly.
	unit, err = s.State.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.service.Unit("mysql")
	c.Assert(err, ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.service.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.service.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `cannot get unit "pressword/0" from service "mysql": .*`)

	// Check direct state retrieval also fails nicely.
	unit, err = s.State.Unit("mysql")
	c.Assert(err, ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.State.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.State.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `unit "pressword/0" not found`)

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
	// Test that we can still read units when the service is Dying...
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	err = s.service.EnsureDying()
	c.Assert(err, IsNil)
	_, err = s.service.AllUnits()
	c.Assert(err, IsNil)
	_, err = s.service.Unit("mysql/0")
	c.Assert(err, IsNil)

	// ...and when those units are Dying or Dead...
	testWhenDying(c, unit, noErr, noErr, func() error {
		_, err := s.service.AllUnits()
		return err
	}, func() error {
		_, err := s.service.Unit("mysql/0")
		return err
	})

	// ...and that we can even read the empty list of units once the
	// service itself is Dead.
	removeAllUnits(c, s.service)
	err = s.service.EnsureDead()
	_, err = s.service.AllUnits()
	c.Assert(err, IsNil)
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
	err = unit.EnsureDead()
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

func (s *ServiceSuite) TestLifeWithUnits(c *C) {
	unit, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	err = s.service.EnsureDying()
	c.Assert(err, IsNil)
	err = s.service.EnsureDead()
	c.Assert(err, ErrorMatches, `cannot finish termination of service "mysql": service still has units and/or relations`)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.service.EnsureDead()
	c.Assert(err, ErrorMatches, `cannot finish termination of service "mysql": service still has units and/or relations`)
	err = s.service.RemoveUnit(unit)
	c.Assert(err, IsNil)
	err = s.service.EnsureDead()
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestLifeWithRelations(c *C) {
	ep1 := state.Endpoint{"mysql", "ifce", "blah1", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(ep1)
	c.Assert(err, IsNil)

	// Check we can't remove the service.
	err = s.State.RemoveService(s.service)
	c.Assert(err, ErrorMatches, `cannot remove service "mysql": service is not dead`)

	// Set Dying, and check that the relation also becomes Dying.
	err = s.service.EnsureDying()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(err, IsNil)
	c.Assert(rel.Life(), Equals, state.Dying)

	// Check that no new relations can be added.
	ep2 := state.Endpoint{"mysql", "ifce", "blah2", state.RolePeer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(ep2)
	c.Assert(err, ErrorMatches, `cannot add relation "mysql:blah2": service "mysql" is not alive`)

	// Check the service can't yet become Dead.
	err = s.service.EnsureDead()
	c.Assert(err, ErrorMatches, `cannot finish termination of service "mysql": service still has units and/or relations`)

	// Check we still can't remove the service.
	err = s.State.RemoveService(s.service)
	c.Assert(err, ErrorMatches, `cannot remove service "mysql": service is not dead`)

	// Make the relation dead; check the service still can't become Dead.
	err = rel.EnsureDead()
	c.Assert(err, IsNil)
	err = s.service.EnsureDead()
	c.Assert(err, ErrorMatches, `cannot finish termination of service "mysql": service still has units and/or relations`)

	// Check the service still can't be removed.
	err = s.State.RemoveService(s.service)
	c.Assert(err, ErrorMatches, `cannot remove service "mysql": service is not dead`)

	// Remove the relation; check the service can become Dead.
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	err = s.service.EnsureDead()
	c.Assert(err, IsNil)

	// Check we can, at last, remove the service.
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.State.RemoveService(s.service)
	c.Assert(err, ErrorMatches, `cannot remove service "mysql": service is not dead`)
	err = s.service.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(s.service)
	c.Assert(err, IsNil)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `unit "mysql/0" not found`)
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

type unitSlice []*state.Unit

func (m unitSlice) Len() int           { return len(m) }
func (m unitSlice) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m unitSlice) Less(i, j int) bool { return m[i].Name() < m[j].Name() }

var serviceUnitsWatchTests = []struct {
	summary string
	test    func(*C, *state.State, *state.Service)
	changes []string
}{
	{
		"Check initial empty event",
		func(_ *C, _ *state.State, _ *state.Service) {},
		[]string(nil),
	}, {
		"Add a unit",
		func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/0"},
	}, {
		"Add a unit, ignore unrelated change",
		func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			unit0, err := service.Unit("mysql/0")
			c.Assert(err, IsNil)
			err = unit0.SetPublicAddress("what.ever")
			c.Assert(err, IsNil)
		},
		[]string{"mysql/1"},
	}, {
		"Add two units at once",
		func(c *C, s *state.State, service *state.Service) {
			_, err := service.AddUnit()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/2", "mysql/3"},
	}, {
		"Report dying unit",
		func(c *C, s *state.State, service *state.Service) {
			unit0, err := service.Unit("mysql/0")
			c.Assert(err, IsNil)
			err = unit0.EnsureDying()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/0"},
	}, {
		"Report dead unit",
		func(c *C, s *state.State, service *state.Service) {
			unit2, err := service.Unit("mysql/2")
			c.Assert(err, IsNil)
			err = unit2.EnsureDying()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/2"},
	}, {
		"Report multiple dead or dying units",
		func(c *C, s *state.State, service *state.Service) {
			unit0, err := service.Unit("mysql/0")
			c.Assert(err, IsNil)
			err = unit0.EnsureDead()
			c.Assert(err, IsNil)
			unit1, err := service.Unit("mysql/1")
			c.Assert(err, IsNil)
			err = unit1.EnsureDead()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/0", "mysql/1"},
	}, {
		"Report dying unit along with a new, alive unit",
		func(c *C, s *state.State, service *state.Service) {
			unit3, err := service.Unit("mysql/3")
			c.Assert(err, IsNil)
			err = unit3.EnsureDying()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/3", "mysql/4"},
	}, {
		"Report multiple dead units and multiple new, alive, units",
		func(c *C, s *state.State, service *state.Service) {
			unit3, err := service.Unit("mysql/3")
			c.Assert(err, IsNil)
			err = unit3.EnsureDead()
			c.Assert(err, IsNil)
			unit4, err := service.Unit("mysql/4")
			c.Assert(err, IsNil)
			err = unit4.EnsureDead()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/3", "mysql/4", "mysql/5", "mysql/6"},
	}, {
		"Add many, and remove many at once",
		func(c *C, s *state.State, service *state.Service) {
			units := [20]*state.Unit{}
			var err error
			for i := 0; i < len(units); i++ {
				units[i], err = service.AddUnit()
				c.Assert(err, IsNil)
			}
			for i := 10; i < len(units); i++ {
				err = units[i].EnsureDead()
				c.Assert(err, IsNil)
				err = service.RemoveUnit(units[i])
				c.Assert(err, IsNil)
			}
		},
		[]string{"mysql/10", "mysql/11", "mysql/12", "mysql/13", "mysql/14", "mysql/15", "mysql/16", "mysql/7", "mysql/8", "mysql/9"},
	}, {
		"Change many at once",
		func(c *C, s *state.State, service *state.Service) {
			units := [10]*state.Unit{}
			var err error
			for i := 0; i < len(units); i++ {
				units[i], err = service.Unit("mysql/" + fmt.Sprint(i+7))
				c.Assert(err, IsNil)
			}
			for _, unit := range units {
				err = unit.EnsureDying()
				c.Assert(err, IsNil)
			}
			err = units[8].EnsureDead()
			c.Assert(err, IsNil)
			err = units[9].EnsureDead()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/10", "mysql/11", "mysql/12", "mysql/13", "mysql/14", "mysql/15", "mysql/16", "mysql/7", "mysql/8", "mysql/9"},
	}, {
		"Report dead when first seen and also add a new unit",
		func(c *C, s *state.State, service *state.Service) {
			unit, err := service.AddUnit()
			c.Assert(err, IsNil)
			err = unit.EnsureDead()
			c.Assert(err, IsNil)
			_, err = service.AddUnit()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/27", "mysql/28"},
	}, {
		"report only units assigned to this machine",
		func(c *C, s *state.State, service *state.Service) {
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
			unit10, err := service.Unit("mysql/10")
			c.Assert(err, IsNil)
			err = unit10.EnsureDead()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit10)
			c.Assert(err, IsNil)
		},
		[]string{"mysql/10", "mysql/29", "mysql/30"},
	}, {
		"Report previously known machines that are removed",
		func(c *C, s *state.State, service *state.Service) {
			unit30, err := service.Unit("mysql/30")
			c.Assert(err, IsNil)
			err = unit30.EnsureDead()
			c.Assert(err, IsNil)
			err = service.RemoveUnit(unit30)
			c.Assert(err, IsNil)
		},
		[]string{"mysql/30"},
	},
}

func (s *ServiceSuite) TestWatchUnits(c *C) {
	unitWatcher := s.service.WatchUnits()
	defer func() {
		c.Assert(unitWatcher.Stop(), IsNil)
	}()
	all := []string{}
	for i, test := range serviceUnitsWatchTests {
		c.Logf("test %d: %s", i, test.summary)
		test.test(c, s.State, s.service)
		s.State.StartSync()
		all = append(all, test.changes...)
		var got []string
		want := append([]string(nil), test.changes...)
		sort.Strings(want)
		for {
			select {
			case new, ok := <-unitWatcher.Changes():
				c.Assert(ok, Equals, true)
				got = append(got, new...)
				if len(got) < len(want) {
					continue
				}
				sort.Strings(got)
				c.Assert(got, DeepEquals, want)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("did not get change, want: %#v", want)
			}
			break
		}
	}

	// Check that removing units for which we already got a Dead event
	// does not yield any more events.
	for _, uname := range all {
		unit, err := s.State.Unit(uname)
		if state.IsNotFound(err) || unit.Life() != state.Dead {
			continue
		}
		c.Assert(err, IsNil)
		svc, err := s.State.Service(unit.ServiceName())
		c.Assert(err, IsNil)
		err = svc.RemoveUnit(unit)
		c.Assert(err, IsNil)
	}
	s.State.StartSync()
	select {
	case got := <-unitWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	// Stop the watcher and restart it, check that it returns non-nil
	// initial event.
	c.Assert(unitWatcher.Stop(), IsNil)
	unitWatcher = s.service.WatchUnits()
	s.State.StartSync()
	want := []string{"mysql/11", "mysql/12", "mysql/13", "mysql/14", "mysql/2", "mysql/28", "mysql/29", "mysql/5", "mysql/6", "mysql/7", "mysql/8", "mysql/9"}
	select {
	case got, ok := <-unitWatcher.Changes():
		c.Assert(ok, Equals, true)
		sort.Strings(got)
		c.Assert(got, DeepEquals, want)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change, want: %#v", want)
	}

	// ignore unrelated change for non-alive unit.
	unit2, err := s.service.Unit("mysql/2")
	c.Assert(err, IsNil)
	err = unit2.SetPublicAddress("what.ever")
	c.Assert(err, IsNil)
	s.State.StartSync()
	select {
	case got := <-unitWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceSuite) TestWatchRelations(c *C) {
	relationsWatcher := s.service.WatchRelations()
	defer func() {
		c.Assert(relationsWatcher.Stop(), IsNil)
	}()

	assertChange := func(want []int) {
		var got []int
		for {
			select {
			case new := <-relationsWatcher.Changes():
				got = append(got, new...)
				if len(got) < len(want) {
					continue
				}
				sort.Ints(got)
				sort.Ints(want)
				c.Assert(got, DeepEquals, want)
				return
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("expected %#v, got nothing", want)
			}
		}
	}
	assertNoChange := func() {
		select {
		case got := <-relationsWatcher.Changes():
			c.Fatalf("expected nothing, got %#v", got)
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Check initial event, and lack of followup.
	s.State.StartSync()
	assertChange(nil)
	assertNoChange()

	// Add a couple of services, check no changes.
	_, err := s.State.AddService("wp1", s.charm)
	c.Assert(err, IsNil)
	_, err = s.State.AddService("wp2", s.charm)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertNoChange()

	// Add a relation; check change.
	mysqlep := state.Endpoint{"mysql", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	wp1ep := state.Endpoint{"wp1", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(mysqlep, wp1ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{0})
	assertNoChange()

	// Add another relation; check change.
	wp2ep := state.Endpoint{"wp2", "ifce", "baz", state.RoleRequirer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(mysqlep, wp2ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{1})
	assertNoChange()

	// Set relation to dying; check change.
	err = rel.EnsureDying()
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{0})

	// Remove a relation; check change.
	err = rel.EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveRelation(rel)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{0})

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
	select {
	case got := <-relationsWatcher.Changes():
		sort.Ints(got)
		c.Assert(got, DeepEquals, []int{1, 2})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("expected %#v, got nothing", []int{1, 2})
	}
	assertNoChange()

	relations := make([]*state.Relation, 5)
	endpoints := make([]state.Endpoint, 5)
	for i := 0; i < 5; i++ {
		_, err := s.State.AddService("hadoop"+fmt.Sprint(i), s.charm)
		c.Assert(err, IsNil)
		endpoints[i] = state.Endpoint{"hadoop" + fmt.Sprint(i), "ifce", "spam" + fmt.Sprint(i), state.RoleRequirer, charm.ScopeGlobal}
		relations[i], err = s.State.AddRelation(mysqlep, endpoints[i])
		c.Assert(err, IsNil)
	}
	err = relations[4].EnsureDead()
	c.Assert(err, IsNil)
	err = s.State.RemoveRelation(relations[4])
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{3, 4, 5, 6})
	assertNoChange()

	err = relations[0].EnsureDying()
	c.Assert(err, IsNil)
	err = relations[1].EnsureDying()
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{3, 4})
	assertNoChange()

	for i := 0; i < 4; i++ {
		err = relations[i].EnsureDead()
		c.Assert(err, IsNil)
		err = s.State.RemoveRelation(relations[i])
		c.Assert(err, IsNil)
	}
	s.State.StartSync()
	assertChange([]int{3, 4, 5, 6})
	assertNoChange()

	_, err = s.State.AddService("postgresql", s.charm)
	ep := state.Endpoint{"postgresql", "ifce", "spam", state.RoleRequirer, charm.ScopeGlobal}
	_, err = s.State.AddRelation(mysqlep, ep)
	c.Assert(err, IsNil)
	s.State.StartSync()
	assertChange([]int{8})
	assertNoChange()
}

func removeAllUnits(c *C, s *state.Service) {
	us, err := s.AllUnits()
	c.Assert(err, IsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, IsNil)
		err = s.RemoveUnit(u)
		c.Assert(err, IsNil)
	}
}

var watchServiceTests = []struct {
	test    func(m *state.Service) error
	Exposed bool
	Life    state.Life
}{
	{
		test: func(s *state.Service) error {
			return s.SetExposed()
		},
		Exposed: true,
	},
	{
		test: func(s *state.Service) error {
			return s.ClearExposed()
		},
		Exposed: false,
	},
	{
		test: func(s *state.Service) error {
			return s.EnsureDying()
		},
		Life: state.Dying,
	},
}

func (s *ServiceSuite) TestWatchService(c *C) {
	altservice, err := s.State.Service(s.service.Name())
	c.Assert(err, IsNil)
	err = altservice.SetCharm(s.charm, true)
	c.Assert(err, IsNil)
	_, force, err := s.service.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, false)

	w := s.service.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	s.State.StartSync()
	select {
	case svc, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(svc.Name(), Equals, s.service.Name())
		_, force, err := svc.Charm()
		c.Assert(err, IsNil)
		c.Assert(force, Equals, true)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change: %v", s.service)
	}

	for i, test := range watchServiceTests {
		c.Logf("test %d", i)
		err := test.test(s.service)
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case service, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(service.Name(), Equals, s.service.Name())
			c.Assert(service.Life(), Equals, test.Life)
			c.Assert(service.IsExposed(), Equals, test.Exposed)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v %v", test.Exposed, test.Life)
		}
	}
	select {
	case got := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceSuite) TestWatchConfig(c *C) {
	config, err := s.service.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)

	configWatcher := s.service.WatchConfig()
	defer func() {
		c.Assert(configWatcher.Stop(), IsNil)
	}()

	s.State.StartSync()
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change")
	}

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

	s.State.StartSync()
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{"baz": "yadda", "foo": "bar"})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change")
	}

	config.Delete("foo")
	changes, err = config.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []state.ItemChange{{
		Key:      "foo",
		Type:     state.ItemDeleted,
		OldValue: "bar",
	}})

	s.State.StartSync()
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{"baz": "yadda"})
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change")
	}

	select {
	case got := <-configWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
