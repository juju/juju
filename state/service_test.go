// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type ServiceSuite struct {
	ConnSuite
	charm *state.Charm
	mysql *state.Service
}

var _ = gc.Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func(*config.Config) (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.charm = s.AddTestingCharm(c, "mysql")
	s.mysql = s.AddTestingService(c, "mysql", s.charm)
}

func (s *ServiceSuite) TestSetCharm(c *gc.C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)
	url, force := s.mysql.CharmURL()
	c.Assert(url, gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)

	// Add a compatible charm and force it.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)
	err = s.mysql.SetCharm(sch, true)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err = s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	url, force = s.mysql.CharmURL()
	c.Assert(url, gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
}

func (s *ServiceSuite) TestSetCharmPreconditions(c *gc.C) {
	logging := s.AddTestingCharm(c, "logging")
	err := s.mysql.SetCharm(logging, false)
	c.Assert(err, gc.ErrorMatches, "cannot change a service's subordinacy")

	othermysql := s.AddSeriesCharm(c, "mysql", "otherseries")
	err = s.mysql.SetCharm(othermysql, false)
	c.Assert(err, gc.ErrorMatches, "cannot change a service's series")
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
}{{
	summary: "different provider (but no relation yet)",
	meta:    metaDifferentProvider,
}, {
	summary: "different requirer (but no relation yet)",
	meta:    metaDifferentRequirer,
}, {
	summary: "different peer",
	meta:    metaDifferentPeer,
	err:     `cannot upgrade service "fakemysql" to charm "local:quantal/quantal-mysql-5": would break relation "fakemysql:cluster"`,
}, {
	summary: "same relations ok",
	meta:    metaBase,
}, {
	summary: "extra endpoints ok",
	meta:    metaExtraEndpoints,
}}

func (s *ServiceSuite) TestSetCharmChecksEndpointsWithoutRelations(c *gc.C) {
	revno := 2
	ms := s.AddMetaCharm(c, "mysql", metaBase, revno)
	svc := s.AddTestingService(c, "fakemysql", ms)
	err := svc.SetCharm(ms, false)
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range setCharmEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)

		newCh := s.AddMetaCharm(c, "mysql", t.meta, revno+i+1)
		err = svc.SetCharm(newCh, false)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmChecksEndpointsWithRelations(c *gc.C) {
	revno := 2
	providerCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, revno)
	providerSvc := s.AddTestingService(c, "myprovider", providerCharm)
	err := providerSvc.SetCharm(providerCharm, false)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	requirerCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	requirerSvc := s.AddTestingService(c, "myrequirer", requirerCharm)
	err = requirerSvc.SetCharm(requirerCharm, false)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("myprovider:kludge", "myrequirer:kludge")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	baseCharm := s.AddMetaCharm(c, "mysql", metaBase, revno)
	err = providerSvc.SetCharm(baseCharm, false)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "myprovider" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
	err = requirerSvc.SetCharm(baseCharm, false)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "myrequirer" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
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
}{{
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
}}

func (s *ServiceSuite) TestSetCharmConfig(c *gc.C) {
	charms := map[string]*state.Charm{
		stringConfig:    s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:     s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:     s.AddConfigCharm(c, "wordpress", floatConfig, 3),
		newStringConfig: s.AddConfigCharm(c, "wordpress", newStringConfig, 4),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		svc := s.AddTestingService(c, "wordpress", origCh)
		err := svc.UpdateConfigSettings(t.startvalues)
		c.Assert(err, jc.ErrorIsNil)

		newCh := charms[t.endconfig]
		err = svc.SetCharm(newCh, false)
		var expectVals charm.Settings
		var expectCh *state.Charm
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			expectCh = origCh
			expectVals = t.startvalues
		} else {
			c.Assert(err, jc.ErrorIsNil)
			expectCh = newCh
			expectVals = t.endvalues
		}

		sch, _, err := svc.Charm()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sch.URL(), gc.DeepEquals, expectCh.URL())
		settings, err := svc.ConfigSettings()
		c.Assert(err, jc.ErrorIsNil)
		if len(expectVals) == 0 {
			c.Assert(settings, gc.HasLen, 0)
		} else {
			c.Assert(settings, gc.DeepEquals, expectVals)
		}

		err = svc.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ServiceSuite) TestSetCharmWithDyingService(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	_, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.SetCharm(sch, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSequenceUnitIdsAfterDestroy(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.mysql = s.AddTestingService(c, "mysql", s.charm)
	unit, err = s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ServiceSuite) TestSequenceUnitIds(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	unit, err = s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ServiceSuite) TestSetCharmWhenDead(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit()
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)

		// Change the service life to Dead manually, as there's no
		// direct way of doing that otherwise.
		ops := []txn.Op{{
			C:      state.ServicesC,
			Id:     state.DocID(s.State, s.mysql.Name()),
			Update: bson.D{{"$set", bson.D{{"life", state.Dead}}}},
		}}

		err = state.RunTransaction(s.State, ops)
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dead)
	}).Check()

	err := s.mysql.SetCharm(sch, true)
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *ServiceSuite) TestSetCharmWithRemovedService(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)
	err = s.mysql.SetCharm(sch, true)
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *ServiceSuite) TestSetCharmWhenRemoved(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertRemoved(c, s.mysql)
	}).Check()

	err := s.mysql.SetCharm(sch, true)
	c.Assert(err, gc.Equals, state.ErrDead)
}

func (s *ServiceSuite) TestSetCharmWhenDyingIsOK(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)
	}).Check()

	err := s.mysql.SetCharm(sch, true)
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
}

func (s *ServiceSuite) TestSetCharmRetriesWithSameCharmURL(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, s.charm.URL())

				err = s.mysql.SetCharm(sch, false)
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the before hook worked.
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, sch.URL())
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a retry.
			After: func() {
				// Verify it worked after the retry.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsTrue)
				c.Assert(currentCh.URL(), jc.DeepEquals, sch.URL())
			},
		},
	).Check()

	err := s.mysql.SetCharm(sch, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmRetriesWhenOldSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)
	err := s.mysql.SetCharm(oldCh, false)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State,
		func() {
			err := s.mysql.UpdateConfigSettings(charm.Settings{"key": "value"})
			c.Assert(err, jc.ErrorIsNil)
		},
		nil, // Ensure there will be a retry.
	).Check()

	err = s.mysql.SetCharm(newCh, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmRetriesWhenBothOldAndNewSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add two units, which will keep the refcount of oldCh
				// and newCh settings greater than 0, while the service's
				// charm URLs change between oldCh and newCh. Ensure
				// refcounts change as expected.
				unit1, err := s.mysql.AddUnit()
				c.Assert(err, jc.ErrorIsNil)
				unit2, err := s.mysql.AddUnit()
				c.Assert(err, jc.ErrorIsNil)
				err = s.mysql.SetCharm(newCh, false)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				err = unit1.SetCharmURL(newCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				// Update newCh settings, switch to oldCh and update its
				// settings as well.
				err = s.mysql.UpdateConfigSettings(charm.Settings{"key": "value1"})
				c.Assert(err, jc.ErrorIsNil)
				err = s.mysql.SetCharm(oldCh, false)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = unit2.SetCharmURL(oldCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateConfigSettings(charm.Settings{"key": "value2"})
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the second attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: func() {
				// SetCharm has refreshed its cached settings for oldCh
				// and newCh. Change them again to trigger another
				// attempt.
				err := s.mysql.SetCharm(newCh, false)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = s.mysql.UpdateConfigSettings(charm.Settings{"key": "value3"})
				c.Assert(err, jc.ErrorIsNil)
				err = s.mysql.SetCharm(oldCh, false)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateConfigSettings(charm.Settings{"key": "value4"})
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				// Verify the charm and refcounts after the third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, oldCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify the charm and refcounts after the final third attempt.
				err := s.mysql.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsTrue)
				c.Assert(currentCh.URL(), jc.DeepEquals, newCh.URL())
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
			},
		},
	).Check()

	err := s.mysql.SetCharm(newCh, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmRetriesWhenOldBindingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	mysqlKey := state.ServiceGlobalKey(s.mysql.Name())
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, revno+1)
	err := s.mysql.SetCharm(oldCharm, false)
	c.Assert(err, jc.ErrorIsNil)

	oldBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(oldBindings, jc.DeepEquals, map[string]string{
		"server":  network.DefaultSpace,
		"kludge":  network.DefaultSpace,
		"cluster": network.DefaultSpace,
	})
	_, err = s.State.AddSpace("db", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("admin", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	updateBindings := func(updatesMap bson.M) {
		ops := []txn.Op{{
			C:      state.EndpointBindingsC,
			Id:     mysqlKey,
			Update: bson.D{{"$set", updatesMap}},
		}}
		err := state.RunTransaction(s.State, ops)
		c.Assert(err, jc.ErrorIsNil)
	}

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// First change.
				updateBindings(bson.M{
					"bindings.server": "db",
					"bindings.kludge": "admin", // will be removed before newCharm is set.
				})
			},
			After: func() {
				// Second change.
				updateBindings(bson.M{
					"bindings.cluster": "admin",
				})
			},
		},
		jujutxn.TestHook{
			Before: nil, // Ensure there will be a (final) retry.
			After: func() {
				// Verify final bindings.
				newBindings, err := s.mysql.EndpointBindings()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(newBindings, jc.DeepEquals, map[string]string{
					"server":  "db", // from the first change.
					"foo":     network.DefaultSpace,
					"client":  network.DefaultSpace,
					"baz":     network.DefaultSpace,
					"cluster": "admin", // from the second change.
					"just":    network.DefaultSpace,
				})
			},
		},
	).Check()

	err = s.mysql.SetCharm(newCharm, true)
	c.Assert(err, jc.ErrorIsNil)
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
	about:   `empty strings are valid`,
	initial: charm.Settings{"outlook": "positive"},
	update:  charm.Settings{"outlook": "", "title": ""},
	expect:  charm.Settings{"outlook": "", "title": ""},
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

func (s *ServiceSuite) TestUpdateConfigSettings(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUpdateConfigSettingsTests {
		c.Logf("test %d. %s", i, t.about)
		svc := s.AddTestingService(c, "dummy-service", sch)
		if t.initial != nil {
			err := svc.UpdateConfigSettings(t.initial)
			c.Assert(err, jc.ErrorIsNil)
		}
		err := svc.UpdateConfigSettings(t.update)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			settings, err := svc.ConfigSettings()
			c.Assert(err, jc.ErrorIsNil)
			expect := t.expect
			if expect == nil {
				expect = charm.Settings{}
			}
			c.Assert(settings, gc.DeepEquals, expect)
		}
		err = svc.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func assertNoSettingsRef(c *gc.C, st *state.State, svcName string, sch *state.Charm) {
	_, err := state.ServiceSettingsRefCount(st, svcName, sch.URL())
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func assertSettingsRef(c *gc.C, st *state.State, svcName string, sch *state.Charm, refcount int) {
	rc, err := state.ServiceSettingsRefCount(st, svcName, sch.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, refcount)
}

func (s *ServiceSuite) TestSettingsRefCountWorks(c *gc.C) {
	// This test ensures the service settings per charm URL are
	// properly reference counted.
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	svcName := "mywp"

	// Both refcounts are zero initially.
	assertNoSettingsRef(c, s.State, svcName, oldCh)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// svc is using oldCh, so its settings refcount is incremented.
	svc := s.AddTestingService(c, svcName, oldCh)
	assertSettingsRef(c, s.State, svcName, oldCh, 1)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Changing to the same charm does not change the refcount.
	err := svc.SetCharm(oldCh, false)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, svcName, oldCh, 1)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Changing from oldCh to newCh causes the refcount of oldCh's
	// settings to be decremented, while newCh's settings is
	// incremented. Consequently, because oldCh's refcount is 0, the
	// settings doc will be removed.
	err = svc.SetCharm(newCh, false)
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, svcName, oldCh)
	assertSettingsRef(c, s.State, svcName, newCh, 1)

	// The same but newCh swapped with oldCh.
	err = svc.SetCharm(oldCh, false)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, svcName, oldCh, 1)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Adding a unit without a charm URL set does not affect the
	// refcount.
	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	curl, ok := u.CharmURL()
	c.Assert(ok, jc.IsFalse)
	assertSettingsRef(c, s.State, svcName, oldCh, 1)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Setting oldCh as the units charm URL increments oldCh, which is
	// used by svc as well, hence 2.
	err = u.SetCharmURL(oldCh.URL())
	c.Assert(err, jc.ErrorIsNil)
	curl, ok = u.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(curl, gc.DeepEquals, oldCh.URL())
	assertSettingsRef(c, s.State, svcName, oldCh, 2)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// A dead unit does not decrement the refcount.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, svcName, oldCh, 2)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Once the unit is removed, refcount is decremented.
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, svcName, oldCh, 1)
	assertNoSettingsRef(c, s.State, svcName, newCh)

	// Finally, after the service is destroyed and removed (since the
	// last unit's gone), the refcount is again decremented.
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, svcName, oldCh)
	assertNoSettingsRef(c, s.State, svcName, newCh)
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

func (s *ServiceSuite) assertServiceRelations(c *gc.C, svc *state.Service, expectedKeys ...string) []*state.Relation {
	rels, err := svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	if len(rels) == 0 {
		return nil
	}
	relKeys := make([]string, len(expectedKeys))
	for i, rel := range rels {
		relKeys[i] = rel.String()
	}
	sort.Strings(relKeys)
	c.Assert(relKeys, gc.DeepEquals, expectedKeys)
	return rels
}

func (s *ServiceSuite) TestNewPeerRelationsAddedOnUpgrade(c *gc.C) {
	// Original mysql charm has no peer relations.
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoPeersMeta, 3)

	// No relations joined yet.
	s.assertServiceRelations(c, s.mysql)

	err := s.mysql.SetCharm(oldCh, false)
	c.Assert(err, jc.ErrorIsNil)
	s.assertServiceRelations(c, s.mysql, "mysql:cluster")

	err = s.mysql.SetCharm(newCh, false)
	c.Assert(err, jc.ErrorIsNil)
	rels := s.assertServiceRelations(c, s.mysql, "mysql:cluster", "mysql:loadbalancer")

	// Check state consistency by attempting to destroy the service.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
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

func (s *ServiceSuite) TestTag(c *gc.C) {
	c.Assert(s.mysql.Tag().String(), gc.Equals, "service-mysql")
}

func (s *ServiceSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.mysql.Endpoint("mysql")
	c.Assert(err, gc.ErrorMatches, `service "mysql" has no "mysql" relation`)

	jiEP, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("mysql"))

	serverEP, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := s.mysql.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{jiEP, serverEP})
}

func (s *ServiceSuite) TestRiakEndpoints(c *gc.C) {
	riak := s.AddTestingService(c, "myriak", s.AddTestingCharm(c, "riak"))

	_, err := riak.Endpoint("garble")
	c.Assert(err, gc.ErrorMatches, `service "myriak" has no "garble" relation`)

	jiEP, err := riak.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("myriak"))

	ringEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ringEP, gc.DeepEquals, state.Endpoint{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(adminEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	endpointEP, err := riak.Endpoint("endpoint")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpointEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "myriak",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "endpoint",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := riak.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{adminEP, endpointEP, jiEP, ringEP})
}

func (s *ServiceSuite) TestWordpressEndpoints(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	_, err := wordpress.Endpoint("nonsense")
	c.Assert(err, gc.ErrorMatches, `service "wordpress" has no "nonsense" relation`)

	jiEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("wordpress"))

	urlEP, err := wordpress.Endpoint("url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urlEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "http",
			Name:      "url",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	ldEP, err := wordpress.Endpoint("logging-dir")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ldEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-dir",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	mpEP, err := wordpress.Endpoint("monitoring-port")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mpEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "monitoring",
			Name:      "monitoring-port",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeContainer,
		},
	})

	dbEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dbEP, gc.DeepEquals, state.Endpoint{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cacheEP, gc.DeepEquals, state.Endpoint{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{cacheEP, dbEP, jiEP, ldEP, mpEP, urlEP})
}

func (s *ServiceSuite) TestServiceRefresh(c *gc.C) {
	s1, err := s.State.Service(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.SetCharm(s.charm, true)
	c.Assert(err, jc.ErrorIsNil)

	testch, force, err := s1.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(testch.URL(), gc.DeepEquals, s.charm.URL())

	err = s1.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	testch, force, err = s1.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsTrue)
	c.Assert(testch.URL(), gc.DeepEquals, s.charm.URL())

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestServiceExposed(c *gc.C) {
	// Check that querying for the exposed flag works correctly.
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Check that setting and clearing the exposed flag works correctly.
	err := s.mysql.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsTrue)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsFalse)

	// Check that setting and clearing the exposed flag repeatedly does not fail.
	err = s.mysql.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.IsExposed(), jc.IsTrue)

	// Make the service Dying and check that ClearExposed and SetExposed fail.
	// TODO(fwereade): maybe service destruction should always unexpose?
	u, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.SetExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)

	// Remove the service and check that both fail.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
}

func (s *ServiceSuite) TestAddUnit(c *gc.C) {
	// Check that principal units can be added on their own.
	unitZero, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.Name(), gc.Equals, "mysql/0")
	c.Assert(unitZero.IsPrincipal(), jc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), gc.HasLen, 0)
	c.Assert(state.GetUnitEnvUUID(unitZero), gc.Equals, s.State.EnvironUUID())

	unitOne, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitOne.Name(), gc.Equals, "mysql/1")
	c.Assert(unitOne.IsPrincipal(), jc.IsTrue)
	c.Assert(unitOne.SubordinateNames(), gc.HasLen, 0)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)

	// Add a subordinate service and check that units cannot be added directly.
	// to add a subordinate unit.
	subCharm := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingService(c, "logging", subCharm)
	_, err = logging.AddUnit()
	c.Assert(err, gc.ErrorMatches, `cannot add unit to service "logging": service is a subordinate`)

	// Indirectly create a subordinate unit by adding a relation and entering
	// scope as a principal.
	eps, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unitZero)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	subZero, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Check that once it's refreshed unitZero has subordinates.
	err = unitZero.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.SubordinateNames(), gc.DeepEquals, []string{"logging/0"})

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id, gc.Equals, m.Id())
}

func (s *ServiceSuite) TestAddUnitWhenNotAlive(c *gc.C) {
	u, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, gc.ErrorMatches, `cannot add unit to service "mysql": service is not alive`)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, gc.ErrorMatches, `cannot add unit to service "mysql": service "mysql" not found`)
}

func (s *ServiceSuite) TestReadUnit(c *gc.C) {
	_, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Check that retrieving a unit from state works correctly.
	unit, err := s.State.Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.State.Unit("mysql")
	c.Assert(err, gc.ErrorMatches, `"mysql" is not a valid unit name`)
	unit, err = s.State.Unit("mysql/0/0")
	c.Assert(err, gc.ErrorMatches, `"mysql/0/0" is not a valid unit name`)
	unit, err = s.State.Unit("pressword/0")
	c.Assert(err, gc.ErrorMatches, `unit "pressword/0" not found`)

	units, err := s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sortedUnitNames(units), gc.DeepEquals, []string{"mysql/0", "mysql/1"})
}

func (s *ServiceSuite) TestReadUnitWhenDying(c *gc.C) {
	// Test that we can still read units when the service is Dying...
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)

	// ...and when those units are Dying or Dead...
	testWhenDying(c, unit, noErr, noErr, func() error {
		_, err := s.mysql.AllUnits()
		return err
	}, func() error {
		_, err := s.State.Unit("mysql/0")
		return err
	})

	// ...and even, in a very limited way, when the service itself is removed.
	removeAllUnits(c, s.mysql)
	_, err = s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestDestroySimple(c *gc.C) {
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyStillHasUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyOnceHadUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyStaleNonZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyStaleZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)

	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a service with no units in relation scope; check service and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *ServiceSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *ServiceSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err = s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Optionally update the service document to get correct relation counts.
	if refresh {
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel0.Life(), gc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the service are are both removed.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ServiceSuite) TestDestroyQueuesUnitCleanup(c *gc.C) {
	// Add 5 units; block quick-remove of mysql/1 and mysql/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.mysql.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			preventUnitDestroyRemove(c, unit)
		}
	}

	// Check state is clean.
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)

	// Destroy mysql, and check units are not touched.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	// Check a cleanup doc was added.
	dirty, err = s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)

	// Run the cleanup and check the units.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	for i, unit := range units {
		if i%2 != 0 {
			assertLife(c, unit, state.Dying)
		} else {
			assertRemoved(c, unit)
		}
	}

	// Check for queued unit cleanups, and run them.
	dirty, err = s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)

	// Check we're now clean.
	dirty, err = s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}

func (s *ServiceSuite) TestRemoveServiceMachine(c *gc.C) {
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	c.Assert(s.mysql.Destroy(), gc.IsNil)
	assertLife(c, s.mysql, state.Dying)

	// Service.Destroy adds units to cleanup, make it happen now.
	c.Assert(s.State.Cleanup(), gc.IsNil)

	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	assertLife(c, machine, state.Dying)
}

func (s *ServiceSuite) TestReadUnitWithChangingState(c *gc.C) {
	// Check that reading a unit after removing the service
	// fails nicely.
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.Unit("mysql/0")
	c.Assert(err, gc.ErrorMatches, `unit "mysql/0" not found`)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func (s *ServiceSuite) TestConstraints(c *gc.C) {
	// Constraints are initially empty (for now).
	cons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons, jc.Satisfies, constraints.IsEmpty)

	// Constraints can be set.
	cons2 := constraints.Value{Mem: uint64p(4096)}
	err = s.mysql.SetConstraints(cons2)
	cons3, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons3, gc.DeepEquals, cons2)

	// Constraints are completely overwritten when re-set.
	cons4 := constraints.Value{CpuPower: uint64p(750)}
	err = s.mysql.SetConstraints(cons4)
	c.Assert(err, jc.ErrorIsNil)
	cons5, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons5, gc.DeepEquals, cons4)

	// Destroy the existing service; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// ...but we can check that old constraints do not affect new services
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons6, jc.Satisfies, constraints.IsEmpty)
}

func (s *ServiceSuite) TestSetInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *ServiceSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw, loggo.DEBUG), gc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting constraints on service "mysql": unsupported constraints: cpu-power`},
	})
	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scons, gc.DeepEquals, cons)
}

func (s *ServiceSuite) TestConstraintsLifecycle(c *gc.C) {
	// Dying.
	unit, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	cons1 := constraints.MustParse("mem=1G")
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: not found or not alive`)
	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&scons, jc.Satisfies, constraints.IsEmpty)

	// Removed (== Dead, for a service).
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: not found or not alive`)
	_, err = s.mysql.Constraints()
	c.Assert(err, gc.ErrorMatches, `constraints not found`)
}

func (s *ServiceSuite) TestSubordinateConstraints(c *gc.C) {
	loggingCh := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingService(c, "logging", loggingCh)

	_, err := logging.Constraints()
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)

	err = logging.SetConstraints(constraints.Value{})
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)
}

func (s *ServiceSuite) TestWatchUnitsBulkEvents(c *gc.C) {
	// Alive unit...
	alive, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Dying unit...
	dying, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dying)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead unit...
	dead, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dead)
	err = dead.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Gone unit.
	gone, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = gone.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// All except gone unit are reported in initial event.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange(alive.Name(), dying.Name(), dead.Name())
	wc.AssertNoChange()

	// Remove them all; alive/dying changes reported; dead never mentioned again.
	err = alive.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = dying.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(alive.Name(), dying.Name())
	wc.AssertNoChange()
}

func (s *ServiceSuite) TestWatchUnitsLifecycle(c *gc.C) {
	// Empty initial event when no units.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Create one unit, check one change.
	quick, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Destroy that unit (short-circuited to removal), check one change.
	err = quick.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Create another, check one change.
	slow, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Change unit itself, no change.
	preventUnitDestroyRemove(c, slow)
	wc.AssertNoChange()

	// Make unit Dying, change detected.
	err = slow.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Make unit Dead, change detected.
	err = slow.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(slow.Name())
	wc.AssertNoChange()

	// Remove unit, final change not detected.
	err = slow.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *ServiceSuite) TestWatchRelations(c *gc.C) {
	// TODO(fwereade) split this test up a bit.
	w := s.mysql.WatchRelations()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Add a relation; check change.
	mysqlep, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	wpch := s.AddTestingCharm(c, "wordpress")
	wpi := 0
	addRelation := func() *state.Relation {
		name := fmt.Sprintf("wp%d", wpi)
		wpi++
		wp := s.AddTestingService(c, name, wpch)
		wpep, err := wp.Endpoint("db")
		c.Assert(err, jc.ErrorIsNil)
		rel, err := s.State.AddRelation(mysqlep, wpep)
		c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	ru2, err := rel2.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Destroy the relation with the unit in scope, and add another; check
	// changes.
	err = rel2.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	rel3 := addRelation()
	wc.AssertChange(rel2.String(), rel3.String())
	wc.AssertNoChange()

	// Leave scope, destroying the relation, and check that change as well.
	err = ru2.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel2.String())
	wc.AssertNoChange()

	// Watch relations on the requirer service too (exercises a
	// different path of the WatchRelations filter function)
	wpx := s.AddTestingService(c, "wpx", wpch)
	wpxWatcher := wpx.WatchRelations()
	defer testing.AssertStop(c, wpxWatcher)
	wpxWatcherC := testing.NewStringsWatcherC(c, s.State, wpxWatcher)
	wpxWatcherC.AssertChange()
	wpxWatcherC.AssertNoChange()

	wpxep, err := wpx.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	relx, err := s.State.AddRelation(mysqlep, wpxep)
	c.Assert(err, jc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()
}

func removeAllUnits(c *gc.C, s *state.Service) {
	us, err := s.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = u.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ServiceSuite) TestWatchService(c *gc.C) {
	w := s.mysql.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	service, err := s.State.Service(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = service.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetCharm(s.charm, true)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove service, start new watch, check single event.
	err = service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	w = s.mysql.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

// SCHEMACHANGE
// TODO(mattyw) remove when schema upgrades are possible
// Check that GetOwnerTag returns user-admin even
// when the service has no owner
func (s *ServiceSuite) TestOwnerTagSchemaProtection(c *gc.C) {
	service := s.AddTestingService(c, "foobar", s.charm)
	state.SetServiceOwnerTag(service, "")
	c.Assert(state.GetServiceOwnerTag(service), gc.Equals, "")
	c.Assert(service.GetOwnerTag(), gc.Equals, "user-admin")
}

func (s *ServiceSuite) TestNetworks(c *gc.C) {
	service, err := s.State.Service(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)
	networks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(networks, gc.HasLen, 0)
}

func (s *ServiceSuite) TestNetworksOnService(c *gc.C) {
	// TODO(dimitern): AddService now ignores networks, as they're deprecated
	// and will be removed in a follow-up. Remove this test then as well.
	networks := []string{"yes", "on"}
	service := s.AddTestingServiceWithNetworks(c, "withnets", s.charm, networks)
	requestedNetworks, err := service.Networks()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(requestedNetworks, gc.HasLen, 0)
	c.Check(requestedNetworks, gc.Not(gc.DeepEquals), networks)
}

func (s *ServiceSuite) TestMetricCredentials(c *gc.C) {
	err := s.mysql.SetMetricCredentials([]byte("hello there"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.MetricCredentials(), gc.DeepEquals, []byte("hello there"))

	service, err := s.State.Service(s.mysql.Name())
	c.Assert(service.MetricCredentials(), gc.DeepEquals, []byte("hello there"))
}

func (s *ServiceSuite) TestMetricCredentialsOnDying(c *gc.C) {
	_, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetMetricCredentials([]byte("set before dying"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.SetMetricCredentials([]byte("set after dying"))
	c.Assert(err, gc.ErrorMatches, "cannot update metric credentials: service not found or not alive")
}

func (s *ServiceSuite) testStatus(c *gc.C, status1, status2, expected state.Status) {
	u1, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.SetStatus(status1, "status 1", nil)
	c.Assert(err, jc.ErrorIsNil)

	u2, err := s.mysql.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	if status2 == state.StatusError {
		err = u2.SetAgentStatus(status2, "status 2", nil)
	} else {
		err = u2.SetStatus(status2, "status 2", nil)
	}
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := s.mysql.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Since, gc.NotNil)
	statusInfo.Since = nil
	c.Assert(statusInfo, jc.DeepEquals, state.StatusInfo{
		Status:  expected,
		Message: "status 2",
		Data:    map[string]interface{}{},
	})
}

func (s *ServiceSuite) TestStatus(c *gc.C) {
	for _, t := range []struct{ status1, status2, expected state.Status }{
		{state.StatusActive, state.StatusWaiting, state.StatusWaiting},
		{state.StatusMaintenance, state.StatusWaiting, state.StatusWaiting},
		{state.StatusActive, state.StatusBlocked, state.StatusBlocked},
		{state.StatusWaiting, state.StatusBlocked, state.StatusBlocked},
		{state.StatusMaintenance, state.StatusBlocked, state.StatusBlocked},
		{state.StatusMaintenance, state.StatusError, state.StatusError},
		{state.StatusBlocked, state.StatusError, state.StatusError},
		{state.StatusWaiting, state.StatusError, state.StatusError},
		{state.StatusActive, state.StatusError, state.StatusError},
	} {
		s.testStatus(c, t.status1, t.status2, t.expected)
	}
}

const oneRequiredStorageMeta = `
storage:
  data0:
    type: block
`

const oneOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
`

const twoRequiredStorageMeta = `
storage:
  data0:
    type: block
  data1:
    type: block
`

const twoOptionalStorageMeta = `
storage:
  data0:
    type: block
    multiple:
      range: 0-
  data1:
    type: block
    multiple:
      range: 0-
`

const oneRequiredFilesystemStorageMeta = `
storage:
  data0:
    type: filesystem
`

const oneRequiredSharedStorageMeta = `
storage:
  data0:
    type: block
    shared: true
`

const oneRequiredReadOnlyStorageMeta = `
storage:
  data0:
    type: block
    read-only: true
`

const oneRequiredLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
`

const oneMultipleLocationStorageMeta = `
storage:
  data0:
    type: filesystem
    location: /srv
    multiple:
      range: 1-
`

func storageRange(min, max int) string {
	var minStr, maxStr string
	if min > 0 {
		minStr = fmt.Sprint(min)
	}
	if max > 0 {
		maxStr = fmt.Sprint(max)
	}
	return fmt.Sprintf(`
    multiple:
      range: %s-%s
`[1:], minStr, maxStr)
}

func (s *ServiceSuite) setCharmFromMeta(c *gc.C, oldMeta, newMeta string) error {
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	svc := s.AddTestingService(c, "test", oldCh)
	return svc.SetCharm(newCh, false)
}

func (s *ServiceSuite) TestSetCharmStorageRemoved(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+twoOptionalStorageMeta,
		mysqlBaseMeta+oneOptionalStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": storage "data1" removed`)
}

func (s *ServiceSuite) TestSetCharmRequiredStorageAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+twoRequiredStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": required storage "data1" added`)
}

func (s *ServiceSuite) TestSetCharmOptionalStorageAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+twoOptionalStorageMeta,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmStorageCountMinDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServiceSuite) TestSetCharmStorageCountMinIncreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" range contracted: min increased from 1 to 2`)
}

func (s *ServiceSuite) TestSetCharmStorageCountMaxDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 2),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 1),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" range contracted: max decreased from 2 to 1`)
}

func (s *ServiceSuite) TestSetCharmStorageCountMaxUnboundedToBounded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, -1),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 999),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" range contracted: max decreased from \<unbounded\> to 999`)
}

func (s *ServiceSuite) TestSetCharmStorageTypeChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" type changed from "block" to "filesystem"`)
}

func (s *ServiceSuite) TestSetCharmStorageSharedChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredSharedStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" shared changed from false to true`)
}

func (s *ServiceSuite) TestSetCharmStorageReadOnlyChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredReadOnlyStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" read-only changed from false to true`)
}

func (s *ServiceSuite) TestSetCharmStorageLocationChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" location changed from "" to "/srv"`)
}

func (s *ServiceSuite) TestSetCharmStorageWithLocationSingletonToMultipleAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
		mysqlBaseMeta+oneMultipleLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade service "test" to charm "mysql": existing storage "data0" with location changed from singleton to multiple`)
}

func (s *ServiceSuite) assertServiceRemovedWithItsBindings(c *gc.C, service *state.Service) {
	// Removing the service removes the bindings with it.
	err := service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	bindings, err := service.EndpointBindings()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(bindings, gc.IsNil)
}

func (s *ServiceSuite) copyBindings(oldMap map[string]string) map[string]string {
	newMap := make(map[string]string, len(oldMap))
	for key, value := range oldMap {
		newMap[key] = value
	}
	return newMap
}

func (s *ServiceSuite) TestEndpointBindingsJustDefaults(c *gc.C) {
	// With unspecified bindings, all endpoints are explicitly bound to the
	// default space when saved in state.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	service := s.AddTestingServiceWithBindings(c, "yoursql", ch, nil)

	setBindings, err := service.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings, jc.DeepEquals, map[string]string{
		"server":  network.DefaultSpace,
		"client":  network.DefaultSpace,
		"cluster": network.DefaultSpace,
	})

	s.assertServiceRemovedWithItsBindings(c, service)
}

func (s *ServiceSuite) TestEndpointBindingsWithExplictOverrides(c *gc.C) {
	_, err := s.State.AddSpace("db", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("ha", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	bindings := map[string]string{
		"server":  "db",
		"cluster": "ha",
	}
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	service := s.AddTestingServiceWithBindings(c, "yoursql", ch, bindings)

	setBindings, err := service.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings, jc.DeepEquals, map[string]string{
		"server":  "db",
		"client":  network.DefaultSpace,
		"cluster": "ha",
	})

	s.assertServiceRemovedWithItsBindings(c, service)
}

func (s *ServiceSuite) TestSetCharmExtraBindingsUseDefaults(c *gc.C) {
	_, err := s.State.AddSpace("db", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 42)
	oldDefaults, err := state.DefaultEndpointBindingsForCharm(oldCharm.Meta())
	c.Assert(err, jc.ErrorIsNil)
	oldBindings := map[string]string{
		"kludge": "db",
		"client": "db",
	}
	service := s.AddTestingServiceWithBindings(c, "yoursql", oldCharm, oldBindings)
	setBindings, err := service.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveOld := s.copyBindings(oldDefaults)
	effectiveOld["kludge"] = "db"
	effectiveOld["client"] = "db"
	c.Assert(setBindings, jc.DeepEquals, effectiveOld)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)
	newDefaults, err := state.DefaultEndpointBindingsForCharm(newCharm.Meta())
	err = service.SetCharm(newCharm, false)
	c.Assert(err, jc.ErrorIsNil)
	setBindings, err = service.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveNew := s.copyBindings(newDefaults)
	effectiveNew["client"] = "db" // "kludge" is missing in newMeta.
	c.Assert(setBindings, jc.DeepEquals, effectiveNew)

	s.assertServiceRemovedWithItsBindings(c, service)
}
