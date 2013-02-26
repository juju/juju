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
	charm *state.Charm
	mysql *state.Service
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "mysql")
	var err error
	s.mysql, err = s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestServiceCharm(c *C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)
	url, force := s.mysql.CharmURL()
	c.Assert(url, DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)

	// TODO: SetCharm must validate the change (version, relations, etc)
	wp := s.AddTestingCharm(c, "wordpress")
	err = s.mysql.SetCharm(wp, true)
	ch, force, err1 := s.mysql.Charm()
	c.Assert(err1, IsNil)
	c.Assert(ch.URL(), DeepEquals, wp.URL())
	c.Assert(force, Equals, true)
	url, force = s.mysql.CharmURL()
	c.Assert(url, DeepEquals, wp.URL())
	c.Assert(force, Equals, true)

	// SetCharm fails when the service is Dying.
	_, err = s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.SetCharm(wp, true)
	c.Assert(err, ErrorMatches, `service "mysql" is not alive`)
}

var stringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
`
var emptyConfig = `
options: {}
`
var floatConfig = `
options:
  key: {default: 0.42, description: Float key, type: float}
`

var setCharmConfigTests = []struct {
	summary     string
	startconfig string
	startvalues map[string]interface{}
	endconfig   string
	endvalues   map[string]interface{}
	err         string
}{
	{
		summary:     "add float key to empty config",
		startconfig: emptyConfig,
		endconfig:   floatConfig,
	}, {
		summary:     "add string key to empty config",
		startconfig: emptyConfig,
		endconfig:   stringConfig,
	}, {
		summary:     "remove string key",
		startconfig: stringConfig,
		startvalues: map[string]interface{}{"key": "value"},
		endconfig:   emptyConfig,
	}, {
		summary:     "remove float key",
		startconfig: floatConfig,
		startvalues: map[string]interface{}{"key": 123.45},
		endconfig:   emptyConfig,
	}, {
		summary:     "change key type without values",
		startconfig: stringConfig,
		endconfig:   floatConfig,
	}, {
		summary:     "change key type with values",
		startconfig: stringConfig,
		startvalues: map[string]interface{}{"key": "value"},
		endconfig:   floatConfig,
		err:         `cannot convert: type of "key" has changed`,
	},
}

func (s *ServiceSuite) TestSetCharmConfig(c *C) {
	charms := map[string]*state.Charm{
		stringConfig: s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:  s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:  s.AddConfigCharm(c, "wordpress", floatConfig, 3),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		svc, err := s.State.AddService("wordpress", origCh)
		c.Assert(err, IsNil)
		cfg, err := svc.Config()
		c.Assert(err, IsNil)
		cfg.Update(t.startvalues)
		_, err = cfg.Write()
		c.Assert(err, IsNil)

		newCh := charms[t.endconfig]
		err = svc.SetCharm(newCh, false)
		var expectVals map[string]interface{}
		var expectCh *state.Charm
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
			expectCh = origCh
			expectVals = t.startvalues
		} else {
			c.Assert(err, IsNil)
			expectCh = newCh
			expectVals = t.endvalues
		}

		sch, _, err := svc.Charm()
		c.Assert(err, IsNil)
		c.Assert(sch.URL(), DeepEquals, expectCh.URL())
		cfg, err = svc.Config()
		c.Assert(err, IsNil)
		if len(expectVals) == 0 {
			c.Assert(cfg.Map(), HasLen, 0)
		} else {
			c.Assert(cfg.Map(), DeepEquals, expectVals)
		}

		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}

func jujuInfoEp(serviceName string) state.Endpoint {
	return state.Endpoint{
		ServiceName:   serviceName,
		Interface:     "juju-info",
		RelationName:  "juju-info",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	}
}

func (s *ServiceSuite) TestMysqlEndpoints(c *C) {
	_, err := s.mysql.Endpoint("mysql")
	c.Assert(err, ErrorMatches, `service "mysql" has no "mysql" relation`)

	jiEP, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, IsNil)
	c.Assert(jiEP, DeepEquals, jujuInfoEp("mysql"))

	serverEP, err := s.mysql.Endpoint("server")
	c.Assert(err, IsNil)
	c.Assert(serverEP, DeepEquals, state.Endpoint{
		ServiceName:   "mysql",
		Interface:     "mysql",
		RelationName:  "server",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	})

	eps, err := s.mysql.Endpoints()
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.Endpoint{jiEP, serverEP})
}

func (s *ServiceSuite) TestRiakEndpoints(c *C) {
	riak, err := s.State.AddService("myriak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)

	_, err = riak.Endpoint("garble")
	c.Assert(err, ErrorMatches, `service "myriak" has no "garble" relation`)

	jiEP, err := riak.Endpoint("juju-info")
	c.Assert(err, IsNil)
	c.Assert(jiEP, DeepEquals, jujuInfoEp("myriak"))

	ringEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	c.Assert(ringEP, DeepEquals, state.Endpoint{
		ServiceName:   "myriak",
		Interface:     "riak",
		RelationName:  "ring",
		RelationRole:  state.RolePeer,
		RelationScope: charm.ScopeGlobal,
	})

	adminEP, err := riak.Endpoint("admin")
	c.Assert(err, IsNil)
	c.Assert(adminEP, DeepEquals, state.Endpoint{
		ServiceName:   "myriak",
		Interface:     "http",
		RelationName:  "admin",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	})

	endpointEP, err := riak.Endpoint("endpoint")
	c.Assert(err, IsNil)
	c.Assert(endpointEP, DeepEquals, state.Endpoint{
		ServiceName:   "myriak",
		Interface:     "http",
		RelationName:  "endpoint",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	})

	eps, err := riak.Endpoints()
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.Endpoint{adminEP, endpointEP, jiEP, ringEP})
}

func (s *ServiceSuite) TestWordpressEndpoints(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)

	_, err = wordpress.Endpoint("nonsense")
	c.Assert(err, ErrorMatches, `service "wordpress" has no "nonsense" relation`)

	jiEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, IsNil)
	c.Assert(jiEP, DeepEquals, jujuInfoEp("wordpress"))

	urlEP, err := wordpress.Endpoint("url")
	c.Assert(err, IsNil)
	c.Assert(urlEP, DeepEquals, state.Endpoint{
		ServiceName:   "wordpress",
		Interface:     "http",
		RelationName:  "url",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeGlobal,
	})

	ldEP, err := wordpress.Endpoint("logging-dir")
	c.Assert(err, IsNil)
	c.Assert(ldEP, DeepEquals, state.Endpoint{
		ServiceName:   "wordpress",
		Interface:     "logging",
		RelationName:  "logging-dir",
		RelationRole:  state.RoleProvider,
		RelationScope: charm.ScopeContainer,
	})

	dbEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	c.Assert(dbEP, DeepEquals, state.Endpoint{
		ServiceName:   "wordpress",
		Interface:     "mysql",
		RelationName:  "db",
		RelationRole:  state.RoleRequirer,
		RelationScope: charm.ScopeGlobal,
	})

	cacheEP, err := wordpress.Endpoint("cache")
	c.Assert(err, IsNil)
	c.Assert(cacheEP, DeepEquals, state.Endpoint{
		ServiceName:   "wordpress",
		Interface:     "varnish",
		RelationName:  "cache",
		RelationRole:  state.RoleRequirer,
		RelationScope: charm.ScopeGlobal,
	})

	eps, err := wordpress.Endpoints()
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.Endpoint{cacheEP, dbEP, jiEP, ldEP, urlEP})
}

func (s *ServiceSuite) TestServiceRefresh(c *C) {
	s1, err := s.State.Service(s.mysql.Name())
	c.Assert(err, IsNil)

	err = s.mysql.SetCharm(s.charm, true)
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

	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *ServiceSuite) TestServiceExposed(c *C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), Equals, false)

	// Check that setting and clearing the exposed flag works correctly.
	err := s.mysql.SetExposed()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.IsExposed(), Equals, true)
	err = s.mysql.ClearExposed()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.IsExposed(), Equals, false)

	// Check that setting and clearing the exposed flag repeatedly does not fail.
	err = s.mysql.SetExposed()
	c.Assert(err, IsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, IsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, IsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, IsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.IsExposed(), Equals, true)

	// Make the service Dying and check that ClearExposed and SetExposed fail.
	// TODO(fwereade): maybe service destruction should always unexpose?
	u, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, ErrorMatches, notAliveErr)
	err = s.mysql.SetExposed()
	c.Assert(err, ErrorMatches, notAliveErr)

	// Remove the service and check that both fail.
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, ErrorMatches, notAliveErr)
	err = s.mysql.ClearExposed()
	c.Assert(err, ErrorMatches, notAliveErr)
}

func (s *ServiceSuite) TestAddUnit(c *C) {
	// Check that principal units can be added on their own.
	unitZero, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "mysql/0")
	c.Assert(unitZero.IsPrincipal(), Equals, true)
	c.Assert(unitZero.SubordinateNames(), HasLen, 0)
	unitOne, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "mysql/1")
	c.Assert(unitOne.IsPrincipal(), Equals, true)
	c.Assert(unitOne.SubordinateNames(), HasLen, 0)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service and check that units cannot be added directly.
	// to add a subordinate unit.
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	_, err = logging.AddUnit()
	c.Assert(err, ErrorMatches, `cannot add unit to service "logging": service is a subordinate`)

	// Indirectly create a subordinate unit by adding a relation and entering
	// scope as a principal.
	eps, err := s.State.InferEndpoints([]string{"logging", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	ru, err := rel.Unit(unitZero)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)
	subZero, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)

	// Check that once it's refreshed unitZero has subordinates.
	err = unitZero.Refresh()
	c.Assert(err, IsNil)
	c.Assert(unitZero.SubordinateNames(), DeepEquals, []string{"logging/0"})

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, m.Id())
}

func (s *ServiceSuite) TestAddUnitWhenNotAlive(c *C) {
	u, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, ErrorMatches, `cannot add unit to service "mysql": service is not alive`)
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, ErrorMatches, `cannot add unit to service "mysql": service "mysql" not found`)
}

func (s *ServiceSuite) TestReadUnit(c *C) {
	_, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving a unit from the service works correctly.
	unit, err := s.mysql.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a unit from state works correctly.
	unit, err = s.State.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.mysql.Unit("mysql")
	c.Assert(err, ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.mysql.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.mysql.Unit("pressword/0")
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
	unit, err = s.mysql.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `cannot get unit "wordpress/0" from service "mysql": .*`)

	units, err := s.mysql.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(sortedUnitNames(units), DeepEquals, []string{"mysql/0", "mysql/1"})
}

func (s *ServiceSuite) TestReadUnitWhenDying(c *C) {
	// Test that we can still read units when the service is Dying...
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	_, err = s.mysql.AllUnits()
	c.Assert(err, IsNil)
	_, err = s.mysql.Unit("mysql/0")
	c.Assert(err, IsNil)

	// ...and when those units are Dying or Dead...
	testWhenDying(c, unit, noErr, noErr, func() error {
		_, err := s.mysql.AllUnits()
		return err
	}, func() error {
		_, err := s.mysql.Unit("mysql/0")
		return err
	})

	// ...and even, in a very limited way, when the service itself is removed.
	removeAllUnits(c, s.mysql)
	_, err = s.mysql.AllUnits()
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestLifeWithUnits(c *C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *ServiceSuite) TestLifeWithRemovableRelations(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	// Destroy a service with no units in relation scope; check service and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, IsNil)
	err = wordpress.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *ServiceSuite) TestLifeWithReferencedRelations(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	// Join a unit to the wordpress side to keep the relation alive.
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)

	// Set Dying, and check that the relation also becomes Dying.
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(err, IsNil)
	c.Assert(rel.Life(), Equals, state.Dying)

	// Check that no new relations can be added.
	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err = s.State.InferEndpoints([]string{"logging", "mysql"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, ErrorMatches, `cannot add relation "logging:info mysql:juju-info": service "mysql" is not alive`)

	// Leave scope on the counterpart side; check the service and relation
	// are both removed.
	err = ru.LeaveScope()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `unit "mysql/0" not found`)
}

func (s *ServiceSuite) TestServiceConfig(c *C) {
	env, err := s.mysql.Config()
	c.Assert(err, IsNil)
	err = env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{})

	env.Update(map[string]interface{}{"spam": "eggs", "eggs": "spam"})
	env.Update(map[string]interface{}{"spam": "spam", "chaos": "emeralds"})
	_, err = env.Write()
	c.Assert(err, IsNil)

	env, err = s.mysql.Config()
	c.Assert(err, IsNil)
	err = env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{"spam": "spam", "eggs": "spam", "chaos": "emeralds"})
}

func (s *ServiceSuite) TestConstraints(c *C) {
	// Constraints are initially empty (for now).
	cons0 := state.Constraints{}
	cons1, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons1, DeepEquals, cons0)

	// Constraints can be set.
	cons2 := state.Constraints{Mem: uint64p(4096)}
	err = s.mysql.SetConstraints(cons2)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons3, DeepEquals, cons2)

	// Constraints are completely overwritten when re-set.
	cons4 := state.Constraints{CpuPower: uint64p(750)}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, IsNil)
	cons5, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons5, DeepEquals, cons4)

	// Destroy the existing service; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)

	// ...but we can check that old constraints do not affect new services
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService(s.mysql.Name(), ch)
	c.Assert(err, IsNil)
	cons6, err := mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons6, DeepEquals, cons0)
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
			err = unit0.Destroy()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/0"},
	}, {
		"Report dead unit",
		func(c *C, s *state.State, service *state.Service) {
			unit2, err := service.Unit("mysql/2")
			c.Assert(err, IsNil)
			err = unit2.Destroy()
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
			err = unit3.Destroy()
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
				err = units[i].Remove()
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
				err = unit.Destroy()
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
			err = unit10.Remove()
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
			err = unit30.Remove()
			c.Assert(err, IsNil)
		},
		[]string{"mysql/30"},
	},
}

func (s *ServiceSuite) TestWatchUnits(c *C) {
	unitWatcher := s.mysql.WatchUnits()
	defer func() {
		c.Assert(unitWatcher.Stop(), IsNil)
	}()
	all := []string{}
	for i, test := range serviceUnitsWatchTests {
		c.Logf("test %d: %s", i, test.summary)
		test.test(c, s.State, s.mysql)
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
		err = unit.Remove()
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
	unitWatcher = s.mysql.WatchUnits()
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
	unit2, err := s.mysql.Unit("mysql/2")
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
	w := s.mysql.WatchRelations()
	defer func() { c.Assert(w.Stop(), IsNil) }()

	assertNoChange := func() {
		s.State.StartSync()
		select {
		case got := <-w.Changes():
			c.Fatalf("expected nothing, got %#v", got)
		case <-time.After(100 * time.Millisecond):
		}
	}
	assertChange := func(want ...int) {
		s.State.Sync()
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			if len(want) == 0 {
				c.Assert(got, HasLen, 0)
			} else {
				sort.Ints(got)
				sort.Ints(want)
				c.Assert(got, DeepEquals, want)
			}
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("expected %#v, got nothing", want)
		}
		assertNoChange()
	}

	// Check initial event, and lack of followup.
	assertChange()

	// Add a relation; check change.
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, IsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wpi := 0
	addRelation := func() *state.Relation {
		name := fmt.Sprintf("wp%d", wpi)
		wpi++
		wp, err := s.State.AddService(name, wpch)
		c.Assert(err, IsNil)
		wpep, err := wp.Endpoint("db")
		c.Assert(err, IsNil)
		rel, err := s.State.AddRelation(mysqlep, wpep)
		c.Assert(err, IsNil)
		return rel
	}
	rel0 := addRelation()
	assertChange(0)

	// Add another relation; check change.
	addRelation()
	assertChange(1)

	// Destroy a relation; check change.
	err = rel0.Destroy()
	c.Assert(err, IsNil)
	assertChange(0)

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
	rel2 := addRelation()
	w = s.mysql.WatchRelations()
	assertChange(1, 2)

	// Add a unit to the new relation; check no change.
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	ru2, err := rel2.Unit(unit)
	c.Assert(err, IsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, IsNil)
	assertNoChange()

	// Destroy the relation with the unit in scope, and add another; check
	// changes.
	err = rel2.Destroy()
	c.Assert(err, IsNil)
	addRelation()
	assertChange(2, 3)

	// Leave scope, destroying the relation, and check that change as well.
	err = ru2.LeaveScope()
	c.Assert(err, IsNil)
	assertChange(2)
}

func removeAllUnits(c *C, s *state.Service) {
	us, err := s.AllUnits()
	c.Assert(err, IsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, IsNil)
		err = u.Remove()
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
	}, {
		test: func(s *state.Service) error {
			return s.ClearExposed()
		},
		Exposed: false,
	}, {
		test: func(s *state.Service) error {
			if _, err := s.AddUnit(); err != nil {
				return err
			}
			return s.Destroy()
		},
		Life: state.Dying,
	},
}

func (s *ServiceSuite) TestWatchService(c *C) {
	altservice, err := s.State.Service(s.mysql.Name())
	c.Assert(err, IsNil)
	err = altservice.SetCharm(s.charm, true)
	c.Assert(err, IsNil)
	_, force, err := s.mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, false)

	w := s.mysql.Watch()
	defer func() {
		c.Assert(w.Stop(), IsNil)
	}()
	s.State.Sync()
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, Equals, true)
		err := s.mysql.Refresh()
		c.Assert(err, IsNil)
		_, force, err := s.mysql.Charm()
		c.Assert(err, IsNil)
		c.Assert(force, Equals, true)
	case <-time.After(500 * time.Millisecond):
		c.Fatalf("did not get change: %v", s.mysql)
	}

	for i, test := range watchServiceTests {
		c.Logf("test %d", i)
		err := test.test(altservice)
		c.Assert(err, IsNil)
		s.State.StartSync()
		select {
		case _, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			err := s.mysql.Refresh()
			c.Assert(err, IsNil)
			c.Assert(s.mysql.Life(), Equals, test.Life)
			c.Assert(s.mysql.IsExposed(), Equals, test.Exposed)
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("did not get change: %v %v", test.Exposed, test.Life)
		}
	}
	s.State.StartSync()
	select {
	case got, ok := <-w.Changes():
		c.Fatalf("got unexpected change: %#v, %v", got, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *ServiceSuite) TestWatchConfig(c *C) {
	config, err := s.mysql.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)

	configWatcher := s.mysql.WatchConfig()
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
