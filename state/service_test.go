// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	"launchpad.net/juju-core/testing/checkers"
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

func (s *ServiceSuite) TestSetCharm(c *C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)
	url, force := s.mysql.CharmURL()
	c.Assert(url, DeepEquals, s.charm.URL())
	c.Assert(force, Equals, false)

	// Add a compatible charm and force it.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2) // revno 1 is used by SetUpSuite
	err = s.mysql.SetCharm(sch, true)
	c.Assert(err, IsNil)
	ch, force, err = s.mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, sch.URL())
	c.Assert(force, Equals, true)
	url, force = s.mysql.CharmURL()
	c.Assert(url, DeepEquals, sch.URL())
	c.Assert(force, Equals, true)

	// SetCharm fails when the service is Dying.
	_, err = s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.SetCharm(sch, true)
	c.Assert(err, ErrorMatches, `service "mysql" is not alive`)
}

func (s *ServiceSuite) TestSetCharmErrors(c *C) {
	logging := s.AddTestingCharm(c, "logging")
	err := s.mysql.SetCharm(logging, false)
	c.Assert(err, ErrorMatches, "cannot change a service's subordinacy")

	othermysql := s.AddSeriesCharm(c, "mysql", "otherseries")
	err = s.mysql.SetCharm(othermysql, false)
	c.Assert(err, ErrorMatches, "cannot change a service's series")
}

var metaBase = `
name: mysql
summary: "Fake MySQL Database engine"
description: "Complete with nonsense relations"
provides:
  server: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaDifferentProvider = `
name: mysql
description: none
summary: none
provides:
  kludge: mysql
requires:
  client: mysql
peers:
  cluster: mysql
`
var metaDifferentRequirer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  kludge: mysql
peers:
  cluster: mysql
`
var metaDifferentPeer = `
name: mysql
description: none
summary: none
provides:
  server: mysql
requires:
  client: mysql
peers:
  kludge: mysql
`
var metaExtraEndpoints = `
name: mysql
description: none
summary: none
provides:
  server: mysql
  foo: bar
requires:
  client: mysql
  baz: woot
peers:
  cluster: mysql
  just: me
`

var setCharmEndpointsTests = []struct {
	summary string
	meta    string
	err     string
}{
	{
		summary: "different provider (but no relation yet)",
		meta:    metaDifferentProvider,
	}, {
		summary: "different requirer (but no relation yet)",
		meta:    metaDifferentRequirer,
	}, {
		summary: "different peer",
		meta:    metaDifferentPeer,
		err:     `cannot upgrade service "fakemysql" to charm "local:series/series-mysql-5": would break relation "fakemysql:cluster"`,
	}, {
		summary: "same relations ok",
		meta:    metaBase,
	}, {
		summary: "extra endpoints ok",
		meta:    metaExtraEndpoints,
	},
}

func (s *ServiceSuite) TestSetCharmChecksEndpointsWithoutRelations(c *C) {
	revno := 2 // 1 is used in SetUpSuite
	ms := s.AddMetaCharm(c, "mysql", metaBase, revno)
	svc, err := s.State.AddService("fakemysql", ms)
	c.Assert(err, IsNil)
	err = svc.SetCharm(ms, false)
	c.Assert(err, IsNil)

	for i, t := range setCharmEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)

		newCh := s.AddMetaCharm(c, "mysql", t.meta, revno+i+1)
		err = svc.SetCharm(newCh, false)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
		}
	}

	err = svc.Destroy()
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestSetCharmChecksEndpointsWithRelations(c *C) {
	revno := 2 // 1 is used by SetUpSuite
	providerCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, revno)
	providerSvc, err := s.State.AddService("myprovider", providerCharm)
	c.Assert(err, IsNil)
	err = providerSvc.SetCharm(providerCharm, false)
	c.Assert(err, IsNil)

	revno++
	requirerCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	requirerSvc, err := s.State.AddService("myrequirer", requirerCharm)
	c.Assert(err, IsNil)
	err = requirerSvc.SetCharm(requirerCharm, false)
	c.Assert(err, IsNil)

	eps, err := s.State.InferEndpoints([]string{"myprovider:kludge", "myrequirer:kludge"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	revno++
	baseCharm := s.AddMetaCharm(c, "mysql", metaBase, revno)
	err = providerSvc.SetCharm(baseCharm, false)
	c.Assert(err, ErrorMatches, `cannot upgrade service "myprovider" to charm "local:series/series-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
	err = requirerSvc.SetCharm(baseCharm, false)
	c.Assert(err, ErrorMatches, `cannot upgrade service "myrequirer" to charm "local:series/series-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
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
var newStringConfig = `
options:
  key: {default: My Key, description: Desc, type: string}
  other: {default: None, description: My Other, type: string}
`

var setCharmConfigTests = []struct {
	summary     string
	startconfig string
	startvalues charm.Settings
	endconfig   string
	endvalues   charm.Settings
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
		summary:     "add string key and preserve existing values",
		startconfig: stringConfig,
		startvalues: charm.Settings{"key": "foo"},
		endconfig:   newStringConfig,
		endvalues:   charm.Settings{"key": "foo"},
	}, {
		summary:     "remove string key",
		startconfig: stringConfig,
		startvalues: charm.Settings{"key": "value"},
		endconfig:   emptyConfig,
	}, {
		summary:     "remove float key",
		startconfig: floatConfig,
		startvalues: charm.Settings{"key": 123.45},
		endconfig:   emptyConfig,
	}, {
		summary:     "change key type without values",
		startconfig: stringConfig,
		endconfig:   floatConfig,
	}, {
		summary:     "change key type with values",
		startconfig: stringConfig,
		startvalues: charm.Settings{"key": "value"},
		endconfig:   floatConfig,
	},
}

func (s *ServiceSuite) TestSetCharmConfig(c *C) {
	charms := map[string]*state.Charm{
		stringConfig:    s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:     s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:     s.AddConfigCharm(c, "wordpress", floatConfig, 3),
		newStringConfig: s.AddConfigCharm(c, "wordpress", newStringConfig, 4),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		svc, err := s.State.AddService("wordpress", origCh)
		c.Assert(err, IsNil)
		err = svc.UpdateConfigSettings(t.startvalues)
		c.Assert(err, IsNil)

		newCh := charms[t.endconfig]
		err = svc.SetCharm(newCh, false)
		var expectVals charm.Settings
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
		settings, err := svc.ConfigSettings()
		c.Assert(err, IsNil)
		if len(expectVals) == 0 {
			c.Assert(settings, HasLen, 0)
		} else {
			c.Assert(settings, DeepEquals, expectVals)
		}

		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}

var serviceUpdateConfigSettingsTests = []struct {
	about   string
	initial charm.Settings
	update  charm.Settings
	expect  charm.Settings
	err     string
}{{
	about:  "unknown option",
	update: charm.Settings{"foo": "bar"},
	err:    `unknown option "foo"`,
}, {
	about:  "bad type",
	update: charm.Settings{"skill-level": "profound"},
	err:    `option "skill-level" expected int, got "profound"`,
}, {
	about:  "set string",
	update: charm.Settings{"outlook": "positive"},
	expect: charm.Settings{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": nil, "title": "sir"},
	expect:  charm.Settings{"title": "sir"},
}, {
	about:  "unset missing string",
	update: charm.Settings{"outlook": nil},
}, {
	about:   `empty strings unset string values`,
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: charm.Settings{"title": "sir"},
	update:  charm.Settings{"username": "admin001"},
	expect:  charm.Settings{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: charm.Settings{"username": "admin001", "title": "sir"},
	update:  charm.Settings{"username": nil, "title": "My Title"},
	expect:  charm.Settings{"title": "My Title"},
}, {
	about:  "non-string type",
	update: charm.Settings{"skill-level": 303},
	expect: charm.Settings{"skill-level": int64(303)},
}, {
	about:   "unset non-string type",
	initial: charm.Settings{"skill-level": 303},
	update:  charm.Settings{"skill-level": nil},
}}

func (s *ServiceSuite) TestUpdateConfigSettings(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUpdateConfigSettingsTests {
		c.Logf("test %d. %s", i, t.about)
		svc, err := s.State.AddService("dummy-service", sch)
		c.Assert(err, IsNil)
		if t.initial != nil {
			err := svc.UpdateConfigSettings(t.initial)
			c.Assert(err, IsNil)
		}
		err = svc.UpdateConfigSettings(t.update)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			settings, err := svc.ConfigSettings()
			c.Assert(err, IsNil)
			expect := t.expect
			if expect == nil {
				expect = charm.Settings{}
			}
			c.Assert(settings, DeepEquals, expect)
		}
		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}

func (s *ServiceSuite) TestSettingsRefCountWorks(c *C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	svcName := "mywp"

	assertNoRef := func(sch *state.Charm) {
		_, err := state.ServiceSettingsRefCount(s.State, svcName, sch.URL())
		c.Assert(err, Equals, mgo.ErrNotFound)
	}
	assertRef := func(sch *state.Charm, refcount int) {
		rc, err := state.ServiceSettingsRefCount(s.State, svcName, sch.URL())
		c.Assert(err, IsNil)
		c.Assert(rc, Equals, refcount)
	}

	assertNoRef(oldCh)
	assertNoRef(newCh)

	svc, err := s.State.AddService(svcName, oldCh)
	c.Assert(err, IsNil)
	assertRef(oldCh, 1)
	assertNoRef(newCh)

	err = svc.SetCharm(oldCh, false)
	c.Assert(err, IsNil)
	assertRef(oldCh, 1)
	assertNoRef(newCh)

	err = svc.SetCharm(newCh, false)
	c.Assert(err, IsNil)
	assertNoRef(oldCh)
	assertRef(newCh, 1)

	err = svc.SetCharm(oldCh, false)
	c.Assert(err, IsNil)
	assertRef(oldCh, 1)
	assertNoRef(newCh)

	u, err := svc.AddUnit()
	c.Assert(err, IsNil)
	curl, ok := u.CharmURL()
	c.Assert(ok, Equals, false)
	assertRef(oldCh, 1)
	assertNoRef(newCh)

	err = u.SetCharmURL(oldCh.URL())
	c.Assert(err, IsNil)
	curl, ok = u.CharmURL()
	c.Assert(ok, Equals, true)
	c.Assert(curl, DeepEquals, oldCh.URL())
	assertRef(oldCh, 2)
	assertNoRef(newCh)

	err = u.EnsureDead()
	c.Assert(err, IsNil)
	assertRef(oldCh, 2)
	assertNoRef(newCh)

	err = u.Remove()
	c.Assert(err, IsNil)
	assertRef(oldCh, 1)
	assertNoRef(newCh)

	err = svc.Destroy()
	c.Assert(err, IsNil)
	assertNoRef(oldCh)
	assertNoRef(newCh)
}

const mysqlBaseMeta = `
name: mysql
summary: "Database engine"
description: "A pretty popular database"
provides:
  server: mysql
`
const onePeerMeta = `
peers:
  cluster: mysql
`
const twoPeersMeta = `
peers:
  cluster: mysql
  loadbalancer: phony
`

func (s *ServiceSuite) assertServiceRelations(c *C, svc *state.Service, expectedKeys ...string) []*state.Relation {
	rels, err := svc.Relations()
	c.Assert(err, IsNil)
	if len(rels) == 0 {
		return nil
	}
	relKeys := make([]string, len(expectedKeys))
	for i, rel := range rels {
		relKeys[i] = rel.String()
	}
	sort.Strings(relKeys)
	c.Assert(relKeys, DeepEquals, expectedKeys)
	return rels
}

func (s *ServiceSuite) TestNewPeerRelationsAddedOnUpgrade(c *C) {
	// Original mysql charm has no peer relations.
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoPeersMeta, 3)

	// No relations joined yet.
	s.assertServiceRelations(c, s.mysql)

	err := s.mysql.SetCharm(oldCh, false)
	c.Assert(err, IsNil)
	s.assertServiceRelations(c, s.mysql, "mysql:cluster")

	err = s.mysql.SetCharm(newCh, false)
	c.Assert(err, IsNil)
	rels := s.assertServiceRelations(c, s.mysql, "mysql:cluster", "mysql:loadbalancer")

	// Check state consistency by attempting to destroy the service.
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	}
}

func jujuInfoEp(serviceName string) state.Endpoint {
	return state.Endpoint{
		ServiceName: serviceName,
		Relation: charm.Relation{
			Interface: "juju-info",
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
}

func (s *ServiceSuite) TestTag(c *C) {
	c.Assert(s.mysql.Tag(), Equals, "service-mysql")
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
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
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
		ServiceName: "myriak",
		Relation: charm.Relation{
			Interface: "riak",
			Name:      "ring",
			Role:      charm.RolePeer,
			Scope:     charm.ScopeGlobal,
			Limit:     1,
		},
	})

	adminEP, err := riak.Endpoint("admin")
	c.Assert(err, IsNil)
	c.Assert(adminEP, DeepEquals, state.Endpoint{
		ServiceName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	endpointEP, err := riak.Endpoint("endpoint")
	c.Assert(err, IsNil)
	c.Assert(endpointEP, DeepEquals, state.Endpoint{
		ServiceName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "endpoint",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
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
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "url",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	ldEP, err := wordpress.Endpoint("logging-dir")
	c.Assert(err, IsNil)
	c.Assert(ldEP, DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-dir",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	dbEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	c.Assert(dbEP, DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     1,
		},
	})

	cacheEP, err := wordpress.Endpoint("cache")
	c.Assert(err, IsNil)
	c.Assert(cacheEP, DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "varnish",
			Name:      "cache",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     2,
			Optional:  true,
		},
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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
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
	preventUnitDestroyRemove(c, unit)
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

func (s *ServiceSuite) TestDestroySimple(c *C) {
	err := s.mysql.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyStillHasUnits(c *C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)

	err = unit.Remove()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyOnceHadUnits(c *C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)

	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyStaleNonZeroUnitCount(c *C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)

	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyStaleZeroUnitCount(c *C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)

	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)

	err = s.mysql.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.mysql.Life(), Equals, state.Dying)

	err = unit.Remove()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyWithRemovableRelation(c *C) {
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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = rel.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyWithReferencedRelation(c *C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *ServiceSuite) TestDestroyWithreferencedRelationStaleCount(c *C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *ServiceSuite) assertDestroyWithReferencedRelation(c *C, refresh bool) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel0, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err = s.State.InferEndpoints([]string{"logging", "mysql"})
	c.Assert(err, IsNil)
	rel1, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, IsNil)

	// Optionally update the service document to get correct relation counts.
	if refresh {
		err = s.mysql.Destroy()
		c.Assert(err, IsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = rel0.Refresh()
	c.Assert(err, IsNil)
	c.Assert(rel0.Life(), Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

	// Drop the last reference to the first relation; check the relation and
	// the service are are both removed.
	err = ru.LeaveScope()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	err = rel0.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *ServiceSuite) TestDestroyQueuesUnitCleanup(c *C) {
	// Add 5 units; block quick-remove of mysql/1 and mysql/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.mysql.AddUnit()
		c.Assert(err, IsNil)
		units[i] = unit
		if i%2 != 0 {
			preventUnitDestroyRemove(c, unit)
		}
	}

	// Check state is clean.
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)

	// Destroy mysql, and check units are not touched.
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	for _, unit := range units {
		assertUnitLife(c, unit, state.Alive)
	}

	// Check a cleanup doc was added.
	dirty, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)

	// Run the cleanup and check the units.
	err = s.State.Cleanup()
	c.Assert(err, IsNil)
	for i, unit := range units {
		if i%2 != 0 {
			assertUnitLife(c, unit, state.Dying)
		} else {
			assertUnitRemoved(c, unit)
		}
	}

	// Check we're now clean.
	dirty, err = s.State.NeedsCleanup()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.mysql.Destroy()
	c.Assert(err, IsNil)
	err = s.mysql.Refresh()
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `unit "mysql/0" not found`)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *ServiceSuite) TestConstraints(c *C) {
	// Constraints are initially empty (for now).
	cons0 := constraints.Value{}
	cons1, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons1, DeepEquals, cons0)

	// Constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(4096)}
	err = s.mysql.SetConstraints(cons2)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons3, DeepEquals, cons2)

	// Constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(750)}
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
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)

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

func (s *ServiceSuite) TestConstraintsLifecycle(c *C) {
	// Dying.
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	cons1 := constraints.MustParse("mem=1G")
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, ErrorMatches, `cannot set constraints: not found or not alive`)
	scons, err := s.mysql.Constraints()
	c.Assert(err, IsNil)
	c.Assert(scons, DeepEquals, constraints.Value{})

	// Removed (== Dead, for a service).
	err = unit.EnsureDead()
	c.Assert(err, IsNil)
	err = unit.Remove()
	c.Assert(err, IsNil)
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, ErrorMatches, `cannot set constraints: not found or not alive`)
	_, err = s.mysql.Constraints()
	c.Assert(err, ErrorMatches, `constraints not found`)
}

func (s *ServiceSuite) TestSubordinateConstraints(c *C) {
	loggingCh := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", loggingCh)
	c.Assert(err, IsNil)

	_, err = logging.Constraints()
	c.Assert(err, Equals, state.ErrSubordinateConstraints)

	err = logging.SetConstraints(constraints.Value{})
	c.Assert(err, Equals, state.ErrSubordinateConstraints)
}

func (s *ServiceSuite) TestWatchUnitsBulkEvents(c *C) {
	// Alive unit...
	alive, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)

	// Dying unit...
	dying, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	preventUnitDestroyRemove(c, dying)
	err = dying.Destroy()
	c.Assert(err, IsNil)

	// Dead unit...
	dead, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	preventUnitDestroyRemove(c, dead)
	err = dead.Destroy()
	c.Assert(err, IsNil)
	err = dead.EnsureDead()
	c.Assert(err, IsNil)

	// Gone unit.
	gone, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	err = gone.Destroy()
	c.Assert(err, IsNil)

	// All except gone unit are reported in initial event.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Name(), dying.Name(), dead.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, IsNil)
	err = dying.EnsureDead()
	c.Assert(err, IsNil)
	err = dying.Remove()
	c.Assert(err, IsNil)
	err = dead.Remove()
	c.Assert(err, IsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *ServiceSuite) TestWatchUnitsLifecycle(c *C) {
	// Empty initial event when no units.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Create one unit, check one change.
	quick, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Destroy that unit (short-circuited to removal), check one change.
	err = quick.Destroy()
	c.Assert(err, IsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Create another, check one change.
	slow, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Change unit itself, no change.
	preventUnitDestroyRemove(c, slow)
	wc.AssertNoChange()

	// Make unit Dying, change detected.
	err = slow.Destroy()
	c.Assert(err, IsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Make unit Dead, change detected.
	err = slow.EnsureDead()
	c.Assert(err, IsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Remove unit, final change not detected.
	err = slow.Remove()
	c.Assert(err, IsNil)
	wc.AssertNoChange()
}

func (s *ServiceSuite) TestWatchRelations(c *C) {
	// TODO(fwereade) split this test up a bit.
	w := s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

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
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Add another relation; check change.
	rel1 := addRelation()
	wc.AssertChange(rel1.String())
	wc.AssertNoChange()

	// Destroy a relation; check change.
	err = rel0.Destroy()
	c.Assert(err, IsNil)
	wc.AssertChange(rel0.String())
	wc.AssertNoChange()

	// Stop watcher; check change chan is closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Add a new relation; start a new watcher; check initial event.
	rel2 := addRelation()
	w = s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc = testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(rel1.String(), rel2.String())
	wc.AssertNoChange()

	// Add a unit to the new relation; check no change.
	unit, err := s.mysql.AddUnit()
	c.Assert(err, IsNil)
	ru2, err := rel2.Unit(unit)
	c.Assert(err, IsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, IsNil)
	wc.AssertNoChange()

	// Destroy the relation with the unit in scope, and add another; check
	// changes.
	err = rel2.Destroy()
	c.Assert(err, IsNil)
	rel3 := addRelation()
	wc.AssertChange(rel2.String(), rel3.String())
	wc.AssertNoChange()

	// Leave scope, destroying the relation, and check that change as well.
	err = ru2.LeaveScope()
	c.Assert(err, IsNil)
	wc.AssertChange(rel2.String())
	wc.AssertNoChange()
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

func (s *ServiceSuite) TestWatchService(c *C) {
	w := s.mysql.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	service, err := s.State.Service(s.mysql.Name())
	c.Assert(err, IsNil)
	err = service.SetExposed()
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = service.ClearExposed()
	c.Assert(err, IsNil)
	err = service.SetCharm(s.charm, true)
	c.Assert(err, IsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove service, start new watch, check single event.
	err = service.Destroy()
	c.Assert(err, IsNil)
	w = s.mysql.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *ServiceSuite) TestAnnotatorForService(c *C) {
	testAnnotator(c, func() (state.Annotator, error) {
		return s.State.Service("mysql")
	})
}

func (s *ServiceSuite) TestAnnotationRemovalForService(c *C) {
	annotations := map[string]string{"mykey": "myvalue"}
	err := s.mysql.SetAnnotations(annotations)
	c.Assert(err, IsNil)
	err = s.mysql.Destroy()
	c.Assert(err, IsNil)
	ann, err := s.mysql.Annotations()
	c.Assert(err, IsNil)
	c.Assert(ann, DeepEquals, make(map[string]string))
}
