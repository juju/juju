// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/network"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/resource/resourcetesting"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

type ApplicationSuite struct {
	ConnSuite
	charm *state.Charm
	mysql *state.Application
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.policy.GetConstraintsValidator = func() (constraints.Validator, error) {
		validator := constraints.NewValidator()
		validator.RegisterConflicts([]string{constraints.InstanceType}, []string{constraints.Mem})
		validator.RegisterUnsupported([]string{constraints.CpuPower})
		return validator, nil
	}
	s.charm = s.AddTestingCharm(c, "mysql")
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)
}

func (s *ApplicationSuite) assertNeedsCleanup(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
}

func (s *ApplicationSuite) assertNoCleanup(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}

func (s *ApplicationSuite) TestSetCharm(c *gc.C) {
	ch, force, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)
	url, force := s.mysql.CharmURL()
	c.Assert(url, gc.DeepEquals, s.charm.URL())
	c.Assert(force, jc.IsFalse)

	// Add a compatible charm and force it.
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err = s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
	url, force = s.mysql.CharmURL()
	c.Assert(url, gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
}

func (s *ApplicationSuite) TestCAASSetCharm(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS, CloudRegion: "<none>",
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	// Add a compatible charm and force it.
	sch := state.AddCustomCharm(c, st, "gitlab", "metadata.yaml", metaBase, "kubernetes", 2)

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL(), gc.DeepEquals, sch.URL())
	c.Assert(force, jc.IsTrue)
}

func (s *ApplicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *ApplicationSuite) TestSetCharmCharmSettings(c *gc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		ConfigSettings: charm.Settings{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	settings, err := s.mysql.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{"key": "value"}))

	newCh = s.AddConfigCharm(c, "mysql", newStringConfig, 3)
	err = s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		ConfigSettings: charm.Settings{"other": "one"},
	})
	c.Assert(err, jc.ErrorIsNil)

	settings, err = s.mysql.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, jc.DeepEquals, s.combinedSettings(newCh, charm.Settings{
		"key":   "value",
		"other": "one",
	}))
}

func (s *ApplicationSuite) TestSetCharmCharmSettingsInvalid(c *gc.C) {
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, 2)
	err := s.mysql.SetCharm(state.SetCharmConfig{
		Charm:          newCh,
		ConfigSettings: charm.Settings{"key": 123.45},
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-mysql-2": validating config settings: option "key" expected string, got 123.45`)
}

func (s *ApplicationSuite) TestSetCharmLegacy(c *gc.C) {
	chDifferentSeries := state.AddTestingCharmForSeries(c, s.State, "precise", "mysql")

	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		ForceSeries: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:precise/precise-mysql-1": cannot change an application's series`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeries(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm: chDifferentSeries,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "cs:multi-series2-8": only these series are supported: trusty, wily`)
}

func (s *ApplicationSuite) TestClientApplicationSetCharmUnsupportedSeriesForce(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series2")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		ForceSeries: true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	app, err = s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err = app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:multi-series2-8")
}

func (s *ApplicationSuite) TestClientApplicationSetCharmWrongOS(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "application", ch)

	chDifferentSeries := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-windows")
	cfg := state.SetCharmConfig{
		Charm:       chDifferentSeries,
		ForceSeries: true,
	}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "cs:multi-series-windows-1": OS "Ubuntu" not supported by charm`)
}

func (s *ApplicationSuite) TestSetCharmPreconditions(c *gc.C) {
	logging := s.AddTestingCharm(c, "logging")
	cfg := state.SetCharmConfig{Charm: logging}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:quantal/quantal-logging-1": cannot change an application's subordinacy`)

	othermysql := s.AddSeriesCharm(c, "mysql", "bionic")
	cfg2 := state.SetCharmConfig{Charm: othermysql}
	err = s.mysql.SetCharm(cfg2)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "mysql" to charm "local:bionic/bionic-mysql-1": cannot change an application's series`)
}

func (s *ApplicationSuite) TestSetCharmUpdatesBindings(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("client", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	oldCharm := s.AddMetaCharm(c, "mysql", metaBase, 44)

	application, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  "yoursql",
		Charm: oldCharm,
		EndpointBindings: map[string]string{
			"":       "db",
			"server": "db",
			"client": "client",
		}})
	c.Assert(err, jc.ErrorIsNil)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)
	cfg := state.SetCharmConfig{Charm: newCharm}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	updatedBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedBindings, jc.DeepEquals, map[string]string{
		// Existing bindings are preserved.
		"":        "db",
		"server":  "db",
		"client":  "client",
		"cluster": "db", // inherited from defaults in AddApplication.
		// New endpoints use defaults.
		"foo":  "db",
		"baz":  "db",
		"just": "db",
	})
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
	err:     `cannot upgrade application "fakemysql" to charm "local:quantal/quantal-mysql-5": would break relation "fakemysql:cluster"`,
}, {
	summary: "same relations ok",
	meta:    metaBase,
}, {
	summary: "extra endpoints ok",
	meta:    metaExtraEndpoints,
}}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithoutRelations(c *gc.C) {
	revno := 2
	ms := s.AddMetaCharm(c, "mysql", metaBase, revno)
	app := s.AddTestingApplication(c, "fakemysql", ms)
	cfg := state.SetCharmConfig{Charm: ms}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range setCharmEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)

		newCh := s.AddMetaCharm(c, "mysql", t.meta, revno+i+1)
		cfg := state.SetCharmConfig{Charm: newCh}
		err = app.SetCharm(cfg)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmChecksEndpointsWithRelations(c *gc.C) {
	revno := 2
	providerCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, revno)
	providerApp := s.AddTestingApplication(c, "myprovider", providerCharm)

	cfg := state.SetCharmConfig{Charm: providerCharm}
	err := providerApp.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	requirerCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	requirerApp := s.AddTestingApplication(c, "myrequirer", requirerCharm)
	cfg = state.SetCharmConfig{Charm: requirerCharm}
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("myprovider:kludge", "myrequirer:kludge")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	revno++
	baseCharm := s.AddMetaCharm(c, "mysql", metaBase, revno)
	cfg = state.SetCharmConfig{Charm: baseCharm}
	err = providerApp.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "myprovider" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
	err = requirerApp.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "myrequirer" to charm "local:quantal/quantal-mysql-4": would break relation "myrequirer:kludge myprovider:kludge"`)
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

func (s *ApplicationSuite) TestSetCharmConfig(c *gc.C) {
	charms := map[string]*state.Charm{
		stringConfig:    s.AddConfigCharm(c, "wordpress", stringConfig, 1),
		emptyConfig:     s.AddConfigCharm(c, "wordpress", emptyConfig, 2),
		floatConfig:     s.AddConfigCharm(c, "wordpress", floatConfig, 3),
		newStringConfig: s.AddConfigCharm(c, "wordpress", newStringConfig, 4),
	}

	for i, t := range setCharmConfigTests {
		c.Logf("test %d: %s", i, t.summary)

		origCh := charms[t.startconfig]
		app := s.AddTestingApplication(c, "wordpress", origCh)
		err := app.UpdateCharmConfig(t.startvalues)
		c.Assert(err, jc.ErrorIsNil)

		newCh := charms[t.endconfig]
		cfg := state.SetCharmConfig{Charm: newCh}
		err = app.SetCharm(cfg)
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

		sch, _, err := app.Charm()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(sch.URL(), gc.DeepEquals, expectCh.URL())
		settings, err := app.CharmConfig()
		c.Assert(err, jc.ErrorIsNil)
		expected := s.combinedSettings(sch, expectVals)
		if len(expected) == 0 {
			c.Assert(settings, gc.HasLen, 0)
		} else {
			c.Assert(settings, gc.DeepEquals, expected)
		}

		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestSetCharmWithDyingApplication(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSequenceUnitIdsAfterDestroy(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	err = unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	s.mysql = s.AddTestingApplication(c, "mysql", s.charm)
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestSequenceUnitIds(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	unit, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/1")
}

func (s *ApplicationSuite) TestSetCharmWhenDead(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)

		// Change the application life to Dead manually, as there's no
		// direct way of doing that otherwise.
		ops := []txn.Op{{
			C:      state.ApplicationsC,
			Id:     state.DocID(s.State, s.mysql.Name()),
			Update: bson.D{{"$set", bson.D{{"life", state.Dead}}}},
		}}

		err = state.RunTransaction(s.State, ops)
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dead)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(errors.Cause(err), gc.Equals, state.ErrDead)
}

func (s *ApplicationSuite) TestSetCharmWithRemovedApplication(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertRemoved(c, s.mysql)

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}

	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenRemoved(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		err := s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertRemoved(c, s.mysql)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetCharmWhenDyingIsOK(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.mysql.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		assertLife(c, s.mysql, state.Dying)
	}).Check()

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
}

func (s *ApplicationSuite) TestSetCharmRetriesWithSameCharmURL(c *gc.C) {
	sch := s.AddMetaCharm(c, "mysql", metaBase, 2)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				currentCh, force, err := s.mysql.Charm()
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(force, jc.IsFalse)
				c.Assert(currentCh.URL(), jc.DeepEquals, s.charm.URL())

				cfg := state.SetCharmConfig{Charm: sch}
				err = s.mysql.SetCharm(cfg)
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

	cfg := state.SetCharmConfig{
		Charm:      sch,
		ForceUnits: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)
	cfg := state.SetCharmConfig{Charm: oldCh}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State,
		func() {
			err := s.mysql.UpdateCharmConfig(charm.Settings{"key": "value"})
			c.Assert(err, jc.ErrorIsNil)
		},
		nil, // Ensure there will be a retry.
	).Check()

	cfg = state.SetCharmConfig{
		Charm:      newCh,
		ForceUnits: true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenBothOldAndNewSettingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	oldCh := s.AddConfigCharm(c, "mysql", stringConfig, revno)
	newCh := s.AddConfigCharm(c, "mysql", stringConfig, revno+1)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add two units, which will keep the refcount of oldCh
				// and newCh settings greater than 0, while the application's
				// charm URLs change between oldCh and newCh. Ensure
				// refcounts change as expected.
				unit1, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				unit2, err := s.mysql.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				cfg := state.SetCharmConfig{Charm: newCh}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				err = unit1.SetCharmURL(newCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertNoSettingsRef(c, s.State, "mysql", oldCh)
				// Update newCh settings, switch to oldCh and update its
				// settings as well.
				err = s.mysql.UpdateCharmConfig(charm.Settings{"key": "value1"})
				c.Assert(err, jc.ErrorIsNil)
				cfg = state.SetCharmConfig{Charm: oldCh}

				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = unit2.SetCharmURL(oldCh.URL())
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(charm.Settings{"key": "value2"})
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
				cfg := state.SetCharmConfig{Charm: newCh}

				err := s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 2)
				assertSettingsRef(c, s.State, "mysql", oldCh, 1)
				err = s.mysql.UpdateCharmConfig(charm.Settings{"key": "value3"})
				c.Assert(err, jc.ErrorIsNil)

				cfg = state.SetCharmConfig{Charm: oldCh}
				err = s.mysql.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
				assertSettingsRef(c, s.State, "mysql", newCh, 1)
				assertSettingsRef(c, s.State, "mysql", oldCh, 2)
				err = s.mysql.UpdateCharmConfig(charm.Settings{"key": "value4"})
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

	cfg := state.SetCharmConfig{
		Charm:      newCh,
		ForceUnits: true,
	}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmRetriesWhenOldBindingsChanged(c *gc.C) {
	revno := 2 // revno 1 is used by SetUpSuite
	mysqlKey := state.ApplicationGlobalKey(s.mysql.Name())
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentRequirer, revno)
	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, revno+1)

	cfg := state.SetCharmConfig{Charm: oldCharm}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	oldBindings, err := s.mysql.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(oldBindings, jc.DeepEquals, map[string]string{
		"server":  "",
		"kludge":  "",
		"cluster": "",
	})
	_, err = s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("admin", "", nil, false)
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
					"foo":     "",
					"client":  "",
					"baz":     "",
					"cluster": "admin", // from the second change.
					"just":    "",
				})
			},
		},
	).Check()

	cfg = state.SetCharmConfig{
		Charm:      newCharm,
		ForceUnits: true,
	}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

var applicationUpdateCharmConfigTests = []struct {
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

func (s *ApplicationSuite) TestUpdateCharmConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range applicationUpdateCharmConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateCharmConfig(t.initial)
			c.Assert(err, jc.ErrorIsNil)
		}
		err := app.UpdateCharmConfig(t.update)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			settings, err := app.CharmConfig()
			c.Assert(err, jc.ErrorIsNil)
			appConfig := t.expect
			expected := s.combinedSettings(sch, appConfig)
			c.Assert(settings, gc.DeepEquals, expected)
		}
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestUpdateApplicationSeries(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)
	err := app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "trusty")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesToStart(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)
	err := app.UpdateApplicationSeries("precise", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "precise")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSamesSeriesAfterStart(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				err = unit.AssignToNewMachine()
				c.Assert(err, jc.ErrorIsNil)

				ops := []txn.Op{{
					C:      state.ApplicationsC,
					Id:     state.DocID(s.State, "multi-series"),
					Update: bson.D{{"$set", bson.D{{"series", "trusty"}}}},
				}}
				err = state.RunTransaction(s.State, ops)
				c.Assert(err, jc.ErrorIsNil)
			},
			After: func() {
				assertApplicationSeriesUpdate(c, app, "trusty")
			},
		},
	).Check()

	err := app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "trusty")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesFail(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				cfg := state.SetCharmConfig{Charm: v2}
				err := app.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	// Trusty is listed in only version 1 of the charm.
	err := app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, gc.ErrorMatches, "cannot update series for \"multi-series\" to trusty: series \"trusty\" not supported by charm, supported series are: precise,xenial")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesCharmURLChangedSeriesPass(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				v2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-seriesv2")
				cfg := state.SetCharmConfig{Charm: v2}
				err := app.SetCharm(cfg)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	// Xenial is listed in both revisions of the charm.
	err := app.UpdateApplicationSeries("xenial", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "xenial")
}

func (s *ApplicationSuite) setupMultiSeriesUnitWithSubordinate(c *gc.C) (*state.Application, *state.Application) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)
	subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
	subApp := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate", subCh)

	eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)

	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = subApp.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	return app, subApp
}

func assertApplicationSeriesUpdate(c *gc.C, a *state.Application, series string) {
	err := a.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.Series(), gc.Equals, series)
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinate(c *gc.C) {
	app, subApp := s.setupMultiSeriesUnitWithSubordinate(c)
	err := app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "trusty")
	assertApplicationSeriesUpdate(c, subApp, "trusty")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateFail(c *gc.C) {
	app, subApp := s.setupMultiSeriesUnitWithSubordinate(c)
	err := app.UpdateApplicationSeries("xenial", false)
	c.Assert(err, jc.Satisfies, state.IsIncompatibleSeriesError)
	assertApplicationSeriesUpdate(c, app, "precise")
	assertApplicationSeriesUpdate(c, subApp, "precise")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesWithSubordinateForce(c *gc.C) {
	app, subApp := s.setupMultiSeriesUnitWithSubordinate(c)
	err := app.UpdateApplicationSeries("xenial", true)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "xenial")
	assertApplicationSeriesUpdate(c, subApp, "xenial")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesUnitCountChange(c *gc.C) {
	ch := state.AddTestingCharmMultiSeries(c, s.State, "multi-series")
	app := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series", ch)
	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 0)

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add a subordinate and unit
				subCh := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate")
				_ = state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate", subCh)

				eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := s.State.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)

				unit, err := app.AddUnit(state.AddUnitParams{})
				c.Assert(err, jc.ErrorIsNil)
				err = unit.AssignToNewMachine()
				c.Assert(err, jc.ErrorIsNil)

				ru, err := rel.Unit(unit)
				c.Assert(err, jc.ErrorIsNil)
				err = ru.EnterScope(nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	err = app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "trusty")

	units, err = app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(units), gc.Equals, 1)
	subApp, err := s.State.Application("multi-series-subordinate")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, subApp, "trusty")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinate(c *gc.C) {
	app, subApp := s.setupMultiSeriesUnitWithSubordinate(c)
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), gc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				subCh2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate2")
				subApp2 := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate2", subCh2)
				c.Assert(subApp2.Series(), gc.Equals, "precise")

				eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate2")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := s.State.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)

				err = unit.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				relUnit, err := rel.Unit(unit)
				c.Assert(err, jc.ErrorIsNil)
				err = relUnit.EnterScope(nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	err = app.UpdateApplicationSeries("trusty", false)
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, app, "trusty")
	assertApplicationSeriesUpdate(c, subApp, "trusty")

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, subApp2, "trusty")
}

func (s *ApplicationSuite) TestUpdateApplicationSeriesSecondSubordinateIncompatible(c *gc.C) {
	app, subApp := s.setupMultiSeriesUnitWithSubordinate(c)
	unit, err := s.State.Unit("multi-series/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.SubordinateNames(), gc.DeepEquals, []string{"multi-series-subordinate/0"})

	defer state.SetTestHooks(c, s.State,
		jujutxn.TestHook{
			Before: func() {
				// Add 2nd subordinate
				subCh2 := state.AddTestingCharmMultiSeries(c, s.State, "multi-series-subordinate2")
				subApp2 := state.AddTestingApplicationForSeries(c, s.State, "precise", "multi-series-subordinate2", subCh2)
				c.Assert(subApp2.Series(), gc.Equals, "precise")

				eps, err := s.State.InferEndpoints("multi-series", "multi-series-subordinate2")
				c.Assert(err, jc.ErrorIsNil)
				rel, err := s.State.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)

				err = unit.Refresh()
				c.Assert(err, jc.ErrorIsNil)
				relUnit, err := rel.Unit(unit)
				c.Assert(err, jc.ErrorIsNil)
				err = relUnit.EnterScope(nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	).Check()

	err = app.UpdateApplicationSeries("yakkety", false)
	c.Assert(err, jc.Satisfies, state.IsIncompatibleSeriesError)
	assertApplicationSeriesUpdate(c, app, "precise")
	assertApplicationSeriesUpdate(c, subApp, "precise")

	subApp2, err := s.State.Application("multi-series-subordinate2")
	c.Assert(err, jc.ErrorIsNil)
	assertApplicationSeriesUpdate(c, subApp2, "precise")
}

func assertNoSettingsRef(c *gc.C, st *state.State, appName string, sch *state.Charm) {
	_, err := state.ApplicationSettingsRefCount(st, appName, sch.URL())
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotFound)
}

func assertSettingsRef(c *gc.C, st *state.State, appName string, sch *state.Charm, refcount int) {
	rc, err := state.ApplicationSettingsRefCount(st, appName, sch.URL())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, refcount)
}

func (s *ApplicationSuite) TestSettingsRefCountWorks(c *gc.C) {
	// This test ensures the application settings per charm URL are
	// properly reference counted.
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	// Both refcounts are zero initially.
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// app is using oldCh, so its settings refcount is incremented.
	app := s.AddTestingApplication(c, appName, oldCh)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing to the same charm does not change the refcount.
	cfg := state.SetCharmConfig{Charm: oldCh}
	err := app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Changing from oldCh to newCh causes the refcount of oldCh's
	// settings to be decremented, while newCh's settings is
	// incremented. Consequently, because oldCh's refcount is 0, the
	// settings doc will be removed.
	cfg = state.SetCharmConfig{Charm: newCh}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertSettingsRef(c, s.State, appName, newCh, 1)

	// The same but newCh swapped with oldCh.
	cfg = state.SetCharmConfig{Charm: oldCh}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Adding a unit without a charm URL set does not affect the
	// refcount.
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	curl, ok := u.CharmURL()
	c.Assert(ok, jc.IsFalse)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Setting oldCh as the units charm URL increments oldCh, which is
	// used by app as well, hence 2.
	err = u.SetCharmURL(oldCh.URL())
	c.Assert(err, jc.ErrorIsNil)
	curl, ok = u.CharmURL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(curl, gc.DeepEquals, oldCh.URL())
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// A dead unit does not decrement the refcount.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 2)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Once the unit is removed, refcount is decremented.
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Finally, after the application is destroyed and removed (since the
	// last unit's gone), the refcount is again decremented.
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoSettingsRef(c, s.State, appName, oldCh)
	assertNoSettingsRef(c, s.State, appName, newCh)

	// Having studiously avoided triggering cleanups throughout,
	// invoke them now and check that the charms are cleaned up
	// correctly -- and that a storm of cleanups for the same
	// charm are not a problem.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)
	err = oldCh.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = newCh.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSettingsRefCreateRace(c *gc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// just before setting the unit charm url, switch the application
	// away from the original charm, causing the attempt to fail
	// (because the settings have gone away; it's the unit's job to
	// fail out and handle the new charm when it comes back up
	// again).
	dropSettings := func() {
		cfg := state.SetCharmConfig{Charm: newCh}
		err = app.SetCharm(cfg)
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, dropSettings).Check()

	err = unit.SetCharmURL(oldCh.URL())
	c.Check(err, gc.ErrorMatches, "settings reference: does not exist")
}

func (s *ApplicationSuite) TestSettingsRefRemoveRace(c *gc.C) {
	oldCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 1)
	newCh := s.AddConfigCharm(c, "wordpress", emptyConfig, 2)
	appName := "mywp"

	app := s.AddTestingApplication(c, appName, oldCh)
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// just before updating the app charm url, set that charm url on
	// a unit to block the removal.
	grabReference := func() {
		err := unit.SetCharmURL(oldCh.URL())
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, grabReference).Check()

	cfg := state.SetCharmConfig{Charm: newCh}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// check refs to both settings exist
	assertSettingsRef(c, s.State, appName, oldCh, 1)
	assertSettingsRef(c, s.State, appName, newCh, 1)
}

func assertNoOffersRef(c *gc.C, st *state.State, appName string) {
	_, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsNotFound)
}

func assertOffersRef(c *gc.C, st *state.State, appName string, refcount int) {
	rc, err := state.ApplicationOffersRefCount(st, appName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, refcount)
}

func (s *ApplicationSuite) TestOffersRefCountWorks(c *gc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	// Once the offer is removed, refcount is decremented.
	err = ao.Remove("hosted-mysql", false)
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	// Trying to destroy the app while there is an offer fails.
	err = s.mysql.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy application "mysql": application is used by 1 offer`)
	assertOffersRef(c, s.State, "mysql", 1)

	// Remove the last offer and the app can be destroyed.
	err = ao.Remove("mysql-offer", false)
	c.Assert(err, jc.ErrorIsNil)
	assertNoOffersRef(c, s.State, "mysql")

	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertNoOffersRef(c, s.State, "mysql")
}

func (s *ApplicationSuite) TestDestroyApplicationRemoveOffers(c *gc.C) {
	// Refcounts are zero initially.
	assertNoOffersRef(c, s.State, "mysql")

	ao := state.NewApplicationOffers(s.State)
	_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 1)

	_, err = ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "mysql-offer",
		ApplicationName: "mysql",
		Endpoints:       map[string]string{"server": "server"},
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertOffersRef(c, s.State, "mysql", 2)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)

	assertNoOffersRef(c, s.State, "mysql")

	offers, err := ao.AllApplicationOffers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestOffersRefRace(c *gc.C) {
	addOffer := func() {
		ao := state.NewApplicationOffers(s.State)
		_, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
			OfferName:       "hosted-mysql",
			ApplicationName: "mysql",
			Endpoints:       map[string]string{"server": "server"},
			Owner:           s.Owner.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	defer state.SetBeforeHooks(c, s.State, addOffer).Check()

	err := s.mysql.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy application "mysql": application is used by 1 offer`)
	assertOffersRef(c, s.State, "mysql", 1)
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

func (s *ApplicationSuite) assertApplicationRelations(c *gc.C, app *state.Application, expectedKeys ...string) []*state.Relation {
	rels, err := app.Relations()
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

func (s *ApplicationSuite) TestNewPeerRelationsAddedOnUpgrade(c *gc.C) {
	// Original mysql charm has no peer relations.
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+onePeerMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoPeersMeta, 3)

	// No relations joined yet.
	s.assertApplicationRelations(c, s.mysql)

	cfg := state.SetCharmConfig{Charm: oldCh}
	err := s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:cluster")

	cfg = state.SetCharmConfig{Charm: newCh}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	rels := s.assertApplicationRelations(c, s.mysql, "mysql:cluster", "mysql:loadbalancer")

	// Check state consistency by attempting to destroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Check the peer relations got destroyed as well.
	for _, rel := range rels {
		err = rel.Refresh()
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func jujuInfoEp(applicationname string) state.Endpoint {
	return state.Endpoint{
		ApplicationName: applicationname,
		Relation: charm.Relation{
			Interface: "juju-info",
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
}

func (s *ApplicationSuite) TestTag(c *gc.C) {
	c.Assert(s.mysql.Tag().String(), gc.Equals, "application-mysql")
}

func (s *ApplicationSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.mysql.Endpoint("mysql")
	c.Assert(err, gc.ErrorMatches, `application "mysql" has no "mysql" relation`)

	jiEP, err := s.mysql.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("mysql"))

	serverEP, err := s.mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})
	serverAdminEP, err := s.mysql.Endpoint("server-admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverAdminEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "server-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	eps, err := s.mysql.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, jc.SameContents, []state.Endpoint{jiEP, serverEP, serverAdminEP})
}

func (s *ApplicationSuite) TestRiakEndpoints(c *gc.C) {
	riak := s.AddTestingApplication(c, "myriak", s.AddTestingCharm(c, "riak"))

	_, err := riak.Endpoint("garble")
	c.Assert(err, gc.ErrorMatches, `application "myriak" has no "garble" relation`)

	jiEP, err := riak.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("myriak"))

	ringEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ringEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "myriak",
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
		ApplicationName: "myriak",
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
		ApplicationName: "myriak",
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

func (s *ApplicationSuite) TestWordpressEndpoints(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	_, err := wordpress.Endpoint("nonsense")
	c.Assert(err, gc.ErrorMatches, `application "wordpress" has no "nonsense" relation`)

	jiEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jiEP, gc.DeepEquals, jujuInfoEp("wordpress"))

	urlEP, err := wordpress.Endpoint("url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urlEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "wordpress",
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
		ApplicationName: "wordpress",
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
		ApplicationName: "wordpress",
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
		ApplicationName: "wordpress",
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
		ApplicationName: "wordpress",
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

func (s *ApplicationSuite) TestApplicationRefresh(c *gc.C) {
	s1, err := s.State.Application(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:      s.charm,
		ForceUnits: true,
	}

	err = s.mysql.SetCharm(cfg)
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

func (s *ApplicationSuite) TestSetPassword(c *gc.C) {
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Application(s.mysql.Name())
	})
}

func (s *ApplicationSuite) TestApplicationExposed(c *gc.C) {
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

	// Make the application Dying and check that ClearExposed and SetExposed fail.
	// TODO(fwereade): maybe application destruction should always unexpose?
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.SetExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)

	// Remove the application and check that both fail.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
	err = s.mysql.ClearExposed()
	c.Assert(err, gc.ErrorMatches, notAliveErr)
}

func (s *ApplicationSuite) TestAddUnit(c *gc.C) {
	// Check that principal units can be added on their own.
	unitZero, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.Name(), gc.Equals, "mysql/0")
	c.Assert(unitZero.IsPrincipal(), jc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), gc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), gc.Equals, s.State.ModelUUID())

	unitOne, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitOne.Name(), gc.Equals, "mysql/1")
	c.Assert(unitOne.IsPrincipal(), jc.IsTrue)
	c.Assert(unitOne.SubordinateNames(), gc.HasLen, 0)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)

	// Add a subordinate application and check that units cannot be added directly.
	// to add a subordinate unit.
	subCharm := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", subCharm)
	_, err = logging.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "logging": application is a subordinate`)

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

func (s *ApplicationSuite) TestAddUnitWhenNotAlive(c *gc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "mysql": application is not found or not alive`)
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.ErrorMatches, `cannot add unit to application "mysql": application "mysql" not found`)
}

func (s *ApplicationSuite) TestAddCAASUnit(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS, CloudRegion: "<none>",
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	unitZero, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitZero.Name(), gc.Equals, "gitlab/0")
	c.Assert(unitZero.IsPrincipal(), jc.IsTrue)
	c.Assert(unitZero.SubordinateNames(), gc.HasLen, 0)
	c.Assert(state.GetUnitModelUUID(unitZero), gc.Equals, st.ModelUUID())

	err = unitZero.SetWorkloadVersion("3.combined")
	c.Assert(err, jc.ErrorIsNil)
	version, err := unitZero.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(version, gc.Equals, "3.combined")

	err = unitZero.SetMeterStatus(state.MeterGreen.String(), "all good")
	c.Assert(err, jc.ErrorIsNil)
	ms, err := unitZero.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ms.Code, gc.Equals, state.MeterGreen)
	c.Assert(ms.Info, gc.Equals, "all good")

	// But they do have status.
	us, err := unitZero.Status()
	c.Assert(err, jc.ErrorIsNil)
	us.Since = nil
	c.Assert(us, jc.DeepEquals, status.StatusInfo{
		Status:  status.Waiting,
		Message: "waiting for container",
		Data:    map[string]interface{}{},
	})
	as, err := unitZero.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(as.Since, gc.NotNil)
	as.Since = nil
	c.Assert(as, jc.DeepEquals, status.StatusInfo{
		Status: status.Allocating,
		Data:   map[string]interface{}{},
	})
}

func (s *ApplicationSuite) TestAgentTools(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS, CloudRegion: "<none>",
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})
	agentTools := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: app.Series(),
	}

	tools, err := app.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools.Version, gc.DeepEquals, agentTools)
}

func (s *ApplicationSuite) TestSetAgentVersion(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Charm: ch})

	agentVersion := version.MustParseBinary("2.0.1-quantal-and64")
	err := app.SetAgentVersion(agentVersion)
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	tools, err := app.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tools.Version, gc.DeepEquals, agentVersion)
}

func (s *ApplicationSuite) TestAddUnitWithProviderIdNonCAASModel(c *gc.C) {
	u, err := s.mysql.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, jc.ErrorIsNil)
	_, err = u.ContainerInfo()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestReadUnit(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestReadUnitWhenDying(c *gc.C) {
	// Test that we can still read units when the application is Dying...
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

	// ...and even, in a very limited way, when the application itself is removed.
	removeAllUnits(c, s.mysql)
	_, err = s.mysql.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroySimple(c *gc.C) {
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.Life(), gc.Equals, state.Dying)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyStillHasUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestDestroyOnceHadUnits(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestDestroyStaleNonZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestDestroyStaleZeroUnitCount(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a application with no units in relation scope; check application and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *ApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *ApplicationSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err = s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Optionally update the application document to get correct relation counts.
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
	// the application are are both removed.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestDestroyQueuesUnitCleanup(c *gc.C) {
	// Add 5 units; block quick-remove of mysql/1 and mysql/3
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
		if i%2 != 0 {
			preventUnitDestroyRemove(c, unit)
		}
	}

	s.assertNoCleanup(c)

	// Destroy mysql, and check units are not touched.
	err := s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		assertLife(c, unit, state.Alive)
	}

	s.assertNeedsCleanup(c)

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
	s.assertNeedsCleanup(c)
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestRemoveApplicationMachine(c *gc.C) {
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.AssignToMachine(machine), gc.IsNil)

	c.Assert(s.mysql.Destroy(), gc.IsNil)
	assertLife(c, s.mysql, state.Dying)

	// Application.Destroy adds units to cleanup, make it happen now.
	c.Assert(s.State.Cleanup(), gc.IsNil)

	c.Assert(unit.Refresh(), jc.Satisfies, errors.IsNotFound)
	assertLife(c, machine, state.Dying)
}

func (s *ApplicationSuite) TestApplicationCleanupRemovesStorageConstraints(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-block")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("loop", 1024, 1),
	}
	app := s.AddTestingApplicationWithStorage(c, "storage-block", ch, storage)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetCharmURL(ch.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(app.Destroy(), gc.IsNil)
	assertLife(c, app, state.Dying)
	assertCleanupCount(c, s.State, 2)

	// These next API calls are normally done by the uniter.
	err = u.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = u.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure storage constraints and settings are now gone.
	_, err = state.AppStorageConstraints(app)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	settings := state.GetApplicationCharmConfig(s.State, app)
	err = settings.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestRemoveQueuesLocalCharmCleanup(c *gc.C) {
	s.assertNoCleanup(c)

	err := s.mysql.Destroy()

	// Check a cleanup doc was added.
	s.assertNeedsCleanup(c)

	// Run the cleanup
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)

	// Check charm removed
	err = s.charm.Refresh()
	c.Check(err, jc.Satisfies, errors.IsNotFound)

	// Check we're now clean.
	s.assertNoCleanup(c)
}

func (s *ApplicationSuite) TestDestroyQueuesResourcesCleanup(c *gc.C) {
	s.assertNoCleanup(c)

	// Add a resource to the application, ensuring it is stored.
	rSt, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	const content = "abc"
	res := resourcetesting.NewCharmResource(c, "blob", content)
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res, strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	storagePath := state.ResourceStoragePath(c, s.State, outRes.ID)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Cleanup should be registered but not yet run.
	s.assertNeedsCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsTrue)

	// Run the cleanup.
	err = s.State.Cleanup()
	c.Assert(err, jc.ErrorIsNil)

	// Check we're now clean.
	s.assertNoCleanup(c)
	c.Assert(state.IsBlobStored(c, s.State, storagePath), jc.IsFalse)
}

func (s *ApplicationSuite) TestDestroyWithPlaceholderResources(c *gc.C) {
	s.assertNoCleanup(c)

	// Add a placeholder resource to the application.
	rSt, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	res := resourcetesting.NewPlaceholderResource(c, "blob", s.mysql.Name())
	outRes, err := rSt.SetResource(s.mysql.Name(), "user", res.Resource, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(outRes.IsPlaceholder(), jc.IsTrue)

	// Detroy the application.
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// No cleanup required for placeholder resources.
	state.AssertNoCleanupsWithKind(c, s.State, "resourceBlob")
}

func (s *ApplicationSuite) TestReadUnitWithChangingState(c *gc.C) {
	// Check that reading a unit after removing the application
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

func (s *ApplicationSuite) TestConstraints(c *gc.C) {
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

	// Destroy the existing application; there's no way to directly assert
	// that the constraints are deleted...
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// ...but we can check that old constraints do not affect new applications
	// with matching names.
	ch, _, err := s.mysql.Charm()
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, s.mysql.Name(), ch)
	cons6, err := mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&cons6, jc.Satisfies, constraints.IsEmpty)
}

func (s *ApplicationSuite) TestSetInvalidConstraints(c *gc.C) {
	cons := constraints.MustParse("mem=4G instance-type=foo")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "instance-type" overlaps with "mem"`)
}

func (s *ApplicationSuite) TestSetUnsupportedConstraintsWarning(c *gc.C) {
	defer loggo.ResetWriters()
	logger := loggo.GetLogger("test")
	logger.SetLogLevel(loggo.DEBUG)
	var tw loggo.TestWriter
	c.Assert(loggo.RegisterWriter("constraints-tester", &tw), gc.IsNil)

	cons := constraints.MustParse("mem=4G cpu-power=10")
	err := s.mysql.SetConstraints(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tw.Log(), jc.LogMatches, jc.SimpleMessages{{
		loggo.WARNING,
		`setting constraints on application "mysql": unsupported constraints: cpu-power`},
	})
	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scons, gc.DeepEquals, cons)
}

func (s *ApplicationSuite) TestConstraintsLifecycle(c *gc.C) {
	// Dying.
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	cons1 := constraints.MustParse("mem=1G")
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: application is not found or not alive`)
	scons, err := s.mysql.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&scons, jc.Satisfies, constraints.IsEmpty)

	// Removed (== Dead, for a application).
	err = unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetConstraints(cons1)
	c.Assert(err, gc.ErrorMatches, `cannot set constraints: application is not found or not alive`)
	_, err = s.mysql.Constraints()
	c.Assert(err, gc.ErrorMatches, `constraints not found`)
}

func (s *ApplicationSuite) TestSubordinateConstraints(c *gc.C) {
	loggingCh := s.AddTestingCharm(c, "logging")
	logging := s.AddTestingApplication(c, "logging", loggingCh)

	_, err := logging.Constraints()
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)

	err = logging.SetConstraints(constraints.Value{})
	c.Assert(err, gc.Equals, state.ErrSubordinateConstraints)
}

func (s *ApplicationSuite) TestWatchUnitsBulkEvents(c *gc.C) {
	// Alive unit...
	alive, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Dying unit...
	dying, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dying)
	err = dying.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// Dead unit...
	dead, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, dead)
	err = dead.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = dead.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Gone unit.
	gone, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestWatchUnitsLifecycle(c *gc.C) {
	// Empty initial event when no units.
	w := s.mysql.WatchUnits()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange()
	wc.AssertNoChange()

	// Create one unit, check one change.
	quick, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Destroy that unit (short-circuited to removal), check one change.
	err = quick.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(quick.Name())
	wc.AssertNoChange()

	// Create another, check one change.
	slow, err := s.mysql.AddUnit(state.AddUnitParams{})
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

func (s *ApplicationSuite) TestWatchRelations(c *gc.C) {
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
		wp := s.AddTestingApplication(c, name, wpch)
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
	unit, err := s.mysql.AddUnit(state.AddUnitParams{})
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

	// Watch relations on the requirer application too (exercises a
	// different path of the WatchRelations filter function)
	wpx := s.AddTestingApplication(c, "wpx", wpch)
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

	err = relx.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	wpxWatcherC.AssertChange(relx.String())
	wpxWatcherC.AssertNoChange()
}

func removeAllUnits(c *gc.C, s *state.Application) {
	us, err := s.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, u := range us {
		err = u.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = u.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationSuite) TestWatchApplication(c *gc.C) {
	w := s.mysql.Watch()
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Make one change (to a separate instance), check one event.
	application, err := s.State.Application(s.mysql.Name())
	c.Assert(err, jc.ErrorIsNil)
	err = application.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Make two changes, check one event.
	err = application.ClearExposed()
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm:      s.charm,
		ForceUnits: true,
	}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Stop, check closed.
	testing.AssertStop(c, w)
	wc.AssertClosed()

	// Remove application, start new watch, check single event.
	err = application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	w = s.mysql.Watch()
	defer testing.AssertStop(c, w)
	testing.NewNotifyWatcherC(c, s.State, w).AssertOneChange()
}

func (s *ApplicationSuite) TestMetricCredentials(c *gc.C) {
	err := s.mysql.SetMetricCredentials([]byte("hello there"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.MetricCredentials(), gc.DeepEquals, []byte("hello there"))

	application, err := s.State.Application(s.mysql.Name())
	c.Assert(application.MetricCredentials(), gc.DeepEquals, []byte("hello there"))
}

func (s *ApplicationSuite) TestMetricCredentialsOnDying(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.SetMetricCredentials([]byte("set before dying"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.SetMetricCredentials([]byte("set after dying"))
	c.Assert(err, gc.ErrorMatches, "cannot update metric credentials: application is not found or not alive")
}

func (s *ApplicationSuite) testStatus(c *gc.C, status1, status2, expected status.Status) {
	u1, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status1,
		Message: "status 1",
		Since:   &now,
	}
	err = u1.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	u2, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status2,
		Message: "status 2",
		Since:   &now,
	}
	if status2 == status.Error {
		err = u2.SetAgentStatus(sInfo)
	} else {
		err = u2.SetStatus(sInfo)
	}
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := s.mysql.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Since, gc.NotNil)
	statusInfo.Since = nil
	c.Assert(statusInfo, jc.DeepEquals, status.StatusInfo{
		Status:  expected,
		Message: "status 2",
		Data:    map[string]interface{}{},
	})
}

func (s *ApplicationSuite) TestStatus(c *gc.C) {
	for _, t := range []struct{ status1, status2, expected status.Status }{
		{status.Active, status.Waiting, status.Waiting},
		{status.Maintenance, status.Waiting, status.Waiting},
		{status.Active, status.Blocked, status.Blocked},
		{status.Waiting, status.Blocked, status.Blocked},
		{status.Maintenance, status.Blocked, status.Blocked},
		{status.Maintenance, status.Error, status.Error},
		{status.Blocked, status.Error, status.Error},
		{status.Waiting, status.Error, status.Error},
		{status.Active, status.Error, status.Error},
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

const oneRequiredOneOptionalStorageMeta = `
storage:
  data0:
    type: block
  data1:
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

const oneOptionalSharedStorageMeta = `
storage:
  data0:
    type: block
    shared: true
    multiple:
      range: 0-
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

func (s *ApplicationSuite) setCharmFromMeta(c *gc.C, oldMeta, newMeta string) error {
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)

	cfg := state.SetCharmConfig{Charm: newCh}
	return app.SetCharm(cfg)
}

func (s *ApplicationSuite) TestSetCharmOptionalUnusedStorageRemoved(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredOneOptionalStorageMeta,
		mysqlBaseMeta+oneRequiredStorageMeta,
	)
	c.Assert(err, jc.ErrorIsNil)
	// It's valid to remove optional storage so long
	// as it is not in use.
}

func (s *ApplicationSuite) TestSetCharmOptionalUsedStorageRemoved(c *gc.C) {
	oldMeta := mysqlBaseMeta + oneRequiredOneOptionalStorageMeta
	newMeta := mysqlBaseMeta + oneRequiredStorageMeta
	oldCh := s.AddMetaCharm(c, "mysql", oldMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", newMeta, 3)
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "test",
		Charm: oldCh,
		Storage: map[string]state.StorageConstraints{
			"data0": {Count: 1},
			"data1": {Count: 1},
		},
	})
	defer state.SetBeforeHooks(c, s.State, func() {
		// Adding a unit will cause the storage to be in-use.
		_, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	cfg := state.SetCharmConfig{Charm: newCh}
	err := app.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": in-use storage "data1" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageRemoved(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": required storage "data0" removed`)
}

func (s *ApplicationSuite) TestSetCharmRequiredStorageAddedDefaultConstraints(c *gc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoRequiredStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{Charm: newCh}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the new required storage was added for the unit.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 2)
}

func (s *ApplicationSuite) TestSetCharmStorageAddedUserSpecifiedConstraints(c *gc.C) {
	oldCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+oneRequiredStorageMeta, 2)
	newCh := s.AddMetaCharm(c, "mysql", mysqlBaseMeta+twoOptionalStorageMeta, 3)
	app := s.AddTestingApplication(c, "test", oldCh)
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	cfg := state.SetCharmConfig{
		Charm: newCh,
		StorageConstraints: map[string]state.StorageConstraints{
			"data1": {Count: 3},
		},
	}
	err = app.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// Check that new storage was added for the unit, based on the
	// constraints specified in SetCharmConfig.
	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	attachments, err := sb.UnitStorageAttachments(u.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.HasLen, 4)
}

func (s *ApplicationSuite) TestSetCharmOptionalStorageAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+twoOptionalStorageMeta,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMinIncreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 3),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(2, 3),
	)
	// User must increase the storage constraints from 1 to 2.
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": validating storage constraints: charm "mysql" store "data0": 2 instances required, 1 specified`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxDecreased(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 2),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 1),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from 2 to 1`)
}

func (s *ApplicationSuite) TestSetCharmStorageCountMaxUnboundedToBounded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, -1),
		mysqlBaseMeta+oneRequiredStorageMeta+storageRange(1, 999),
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" range contracted: max decreased from \<unbounded\> to 999`)
}

func (s *ApplicationSuite) TestSetCharmStorageTypeChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" type changed from "block" to "filesystem"`)
}

func (s *ApplicationSuite) TestSetCharmStorageSharedChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneOptionalStorageMeta,
		mysqlBaseMeta+oneOptionalSharedStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" shared changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageReadOnlyChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredStorageMeta,
		mysqlBaseMeta+oneRequiredReadOnlyStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" read-only changed from false to true`)
}

func (s *ApplicationSuite) TestSetCharmStorageLocationChanged(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredFilesystemStorageMeta,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" location changed from "" to "/srv"`)
}

func (s *ApplicationSuite) TestSetCharmStorageWithLocationSingletonToMultipleAdded(c *gc.C) {
	err := s.setCharmFromMeta(c,
		mysqlBaseMeta+oneRequiredLocationStorageMeta,
		mysqlBaseMeta+oneMultipleLocationStorageMeta,
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "test" to charm "local:quantal/quantal-mysql-3": existing storage "data0" with location changed from single to multiple`)
}

func (s *ApplicationSuite) assertApplicationRemovedWithItsBindings(c *gc.C, application *state.Application) {
	// Removing the application removes the bindings with it.
	err := application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	state.AssertEndpointBindingsNotFoundForApplication(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsReturnsDefaultsWhenNotFound(c *gc.C) {
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)
	state.RemoveEndpointBindingsForApplication(c, application)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
}

func (s *ApplicationSuite) assertApplicationHasOnlyDefaultEndpointBindings(c *gc.C, application *state.Application) {
	charm, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)

	knownEndpoints := set.NewStrings()
	allBindings := state.DefaultEndpointBindingsForCharm(charm.Meta())
	for endpoint := range allBindings {
		knownEndpoints.Add(endpoint)
	}

	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings, gc.NotNil)

	for endpoint, space := range setBindings {
		c.Check(endpoint, gc.Not(gc.Equals), "")
		c.Check(knownEndpoints.Contains(endpoint), jc.IsTrue)
		c.Check(space, gc.Equals, "", gc.Commentf("expected empty space for endpoint %q, got %q", endpoint, space))
	}
}

func (s *ApplicationSuite) TestEndpointBindingsJustDefaults(c *gc.C) {
	// With unspecified bindings, all endpoints are explicitly bound to the
	// default space when saved in state.
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, nil)

	s.assertApplicationHasOnlyDefaultEndpointBindings(c, application)
	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestEndpointBindingsWithExplictOverrides(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("ha", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	bindings := map[string]string{
		"server":  "db",
		"cluster": "ha",
	}
	ch := s.AddMetaCharm(c, "mysql", metaBase, 42)
	application := s.AddTestingApplicationWithBindings(c, "yoursql", ch, bindings)

	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(setBindings, jc.DeepEquals, map[string]string{
		"server":  "db",
		"client":  "",
		"cluster": "ha",
	})

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmExtraBindingsUseDefaults(c *gc.C) {
	_, err := s.State.AddSpace("db", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 42)
	oldBindings := map[string]string{
		"kludge": "db",
		"client": "db",
	}
	application := s.AddTestingApplicationWithBindings(c, "yoursql", oldCharm, oldBindings)
	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveOld := map[string]string{
		"kludge":  "db",
		"client":  "db",
		"cluster": "",
	}
	c.Assert(setBindings, jc.DeepEquals, effectiveOld)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 43)

	cfg := state.SetCharmConfig{Charm: newCharm}
	err = application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	setBindings, err = application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveNew := map[string]string{
		// These two should be preserved from oldCharm.
		"client":  "db",
		"cluster": "",
		// "kludge" is missing in newMeta, "server" is new and gets the default.
		"server": "",
		// All the remaining are new and use the empty default.
		"foo":  "",
		"baz":  "",
		"just": "",
	}
	c.Assert(setBindings, jc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) TestSetCharmHandlesMissingBindingsAsDefaults(c *gc.C) {
	oldCharm := s.AddMetaCharm(c, "mysql", metaDifferentProvider, 69)
	application := s.AddTestingApplicationWithBindings(c, "theirsql", oldCharm, nil)
	state.RemoveEndpointBindingsForApplication(c, application)

	newCharm := s.AddMetaCharm(c, "mysql", metaExtraEndpoints, 70)

	cfg := state.SetCharmConfig{Charm: newCharm}
	err := application.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	setBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	effectiveNew := map[string]string{
		// The following two exist for both oldCharm and newCharm.
		"client":  "",
		"cluster": "",
		// "kludge" is missing in newMeta, "server" is new and gets the default.
		"server": "",
		// All the remaining are new and use the empty default.
		"foo":  "",
		"baz":  "",
		"just": "",
	}
	c.Assert(setBindings, jc.DeepEquals, effectiveNew)

	s.assertApplicationRemovedWithItsBindings(c, application)
}

func (s *ApplicationSuite) setupAppicationWithUnitsForUpgradeCharmScenario(c *gc.C, numOfUnits int) (deployedV int, err error) {
	originalCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgreplication
`
	originalCharm := s.AddMetaCharm(c, "mysql", originalCharmMeta, 2)
	cfg := state.SetCharmConfig{Charm: originalCharm}
	err = s.mysql.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationRelations(c, s.mysql, "mysql:replication")
	deployedV = s.mysql.CharmModifiedVersion()

	for i := 0; i < numOfUnits; i++ {
		_, err = s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
	}

	// New mysql charm renames peer relation.
	updatedCharmMeta := mysqlBaseMeta + `
peers:
  replication:
    interface: pgpeer
`
	updatedCharm := s.AddMetaCharm(c, "mysql", updatedCharmMeta, 3)

	cfg = state.SetCharmConfig{Charm: updatedCharm}
	err = s.mysql.SetCharm(cfg)
	return
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithOneUnit(c *gc.C) {
	obtainedV, err := s.setupAppicationWithUnitsForUpgradeCharmScenario(c, 1)

	// ensure upgrade happened
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV+1, jc.IsTrue)
}

func (s *ApplicationSuite) TestRenamePeerRelationOnUpgradeWithMoreThanOneUnit(c *gc.C) {
	obtainedV, err := s.setupAppicationWithUnitsForUpgradeCharmScenario(c, 2)

	// ensure upgrade did not happen
	c.Assert(err, gc.ErrorMatches, `*would break relation "mysql:replication"*`)
	c.Assert(s.mysql.CharmModifiedVersion() == obtainedV, jc.IsTrue)
}

func (s *ApplicationSuite) TestWatchCharmConfig(c *gc.C) {
	oldCharm := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "wordpress", oldCharm)
	// Add a unit so when we change the application's charm,
	// the old charm isn't removed (due to a reference).
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetCharmURL(oldCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	w, err := app.WatchCharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	defer testing.AssertStop(c, w)

	// Initial event.
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = app.UpdateCharmConfig(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = app.UpdateCharmConfig(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = app.UpdateCharmConfig(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application's charm; nothing detected.
	newCharm := s.AddConfigCharm(c, "wordpress", stringConfig, 123)
	err = app.SetCharm(state.SetCharmConfig{Charm: newCharm})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change application config for new charm; nothing detected.
	err = app.UpdateCharmConfig(charm.Settings{"key": "value"})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

var updateApplicationConfigTests = []struct {
	about   string
	initial application.ConfigAttributes
	update  application.ConfigAttributes
	expect  application.ConfigAttributes
	err     string
}{{
	about:  "set string",
	update: application.ConfigAttributes{"outlook": "positive"},
	expect: application.ConfigAttributes{"outlook": "positive"},
}, {
	about:   "unset string and set another",
	initial: application.ConfigAttributes{"outlook": "positive"},
	update:  application.ConfigAttributes{"outlook": nil, "title": "sir"},
	expect:  application.ConfigAttributes{"title": "sir"},
}, {
	about:  "unset missing string",
	update: application.ConfigAttributes{"outlook": nil},
}, {
	about:   `empty strings are valid`,
	initial: application.ConfigAttributes{"outlook": "positive"},
	update:  application.ConfigAttributes{"outlook": "", "title": ""},
	expect:  application.ConfigAttributes{"outlook": "", "title": ""},
}, {
	about:   "preserve existing value",
	initial: application.ConfigAttributes{"title": "sir"},
	update:  application.ConfigAttributes{"username": "admin001"},
	expect:  application.ConfigAttributes{"username": "admin001", "title": "sir"},
}, {
	about:   "unset a default value, set a different default",
	initial: application.ConfigAttributes{"username": "admin001", "title": "sir"},
	update:  application.ConfigAttributes{"username": nil, "title": "My Title"},
	expect:  application.ConfigAttributes{"title": "My Title"},
}, {
	about:  "non-string type",
	update: application.ConfigAttributes{"skill-level": 303},
	expect: application.ConfigAttributes{"skill-level": 303},
}, {
	about:   "unset non-string type",
	initial: application.ConfigAttributes{"skill-level": 303},
	update:  application.ConfigAttributes{"skill-level": nil},
}}

func (s *ApplicationSuite) TestUpdateApplicationConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range updateApplicationConfigTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", sch)
		if t.initial != nil {
			err := app.UpdateApplicationConfig(t.initial, nil, sampleApplicationConfigSchema(), nil)
			c.Assert(err, jc.ErrorIsNil)
		}
		updates := make(map[string]interface{})
		var resets []string
		for k, v := range t.update {
			if v == nil {
				resets = append(resets, k)
			} else {
				updates[k] = v
			}
		}
		err := app.UpdateApplicationConfig(updates, resets, sampleApplicationConfigSchema(), nil)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			cfg, err := app.ApplicationConfig()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cfg, gc.DeepEquals, t.expect)
		}
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func sampleApplicationConfigSchema() environschema.Fields {
	schema := environschema.Fields{
		"title":       environschema.Attr{Type: environschema.Tstring},
		"outlook":     environschema.Attr{Type: environschema.Tstring},
		"username":    environschema.Attr{Type: environschema.Tstring},
		"skill-level": environschema.Attr{Type: environschema.Tint},
	}
	return schema
}

func (s *ApplicationSuite) TestUpdateApplicationConfigWithDyingApplication(c *gc.C) {
	_, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysql.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, s.mysql, state.Dying)
	err = s.mysql.UpdateApplicationConfig(application.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ApplicationSuite) TestDestroyApplicationRemovesConfig(c *gc.C) {
	err := s.mysql.UpdateApplicationConfig(application.ConfigAttributes{"title": "value"}, nil, sampleApplicationConfigSchema(), nil)
	c.Assert(err, jc.ErrorIsNil)
	appConfig := state.GetApplicationConfig(s.State, s.mysql)
	err = appConfig.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appConfig.Map(), gc.Not(gc.HasLen), 0)

	op := s.mysql.DestroyOperation()
	op.RemoveOffers = true
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)
	err = appConfig.Read()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

type CAASApplicationSuite struct {
	ConnSuite
	app    *state.Application
	caasSt *state.State
}

var _ = gc.Suite(&CAASApplicationSuite{})

func (s *CAASApplicationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.caasSt = s.Factory.MakeCAASModel(c, nil)
	s.AddCleanup(func(_ *gc.C) { s.caasSt.Close() })

	f := factory.NewFactory(s.caasSt, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	s.app = f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	// Consume the initial construction events from the watchers.
	s.State.StartSync()
}

func strPtr(s string) *string {
	return &s
}

func (s *CAASApplicationSuite) TestUpdateCAASUnits(c *gc.C) {
	s.assertUpdateCAASUnits(c, true)
}

func (s *CAASApplicationSuite) TestUpdateCAASUnitsApplicationNotALive(c *gc.C) {
	s.assertUpdateCAASUnits(c, false)
}

func (s *CAASApplicationSuite) assertUpdateCAASUnits(c *gc.C, aliveApp bool) {
	existingUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("unit-uuid")})
	c.Assert(err, jc.ErrorIsNil)
	removedUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("removed-unit-uuid")})
	c.Assert(err, jc.ErrorIsNil)
	noContainerUnit, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("never-cloud-container")})
	c.Assert(err, jc.ErrorIsNil)
	if !aliveApp {
		err := s.app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	var updateUnits state.UpdateUnitsOperation
	updateUnits.Deletes = []*state.DestroyUnitOperation{removedUnit.DestroyOperation()}
	updateUnits.Adds = []*state.AddUnitOperation{
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("new-unit-uuid"),
			Address:    strPtr("192.168.1.1"),
			Ports:      &[]string{"80"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new container running",
			},
		}),
		s.app.AddOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("add-never-cloud-container"),
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "new running",
			},
			// Status history should not show this as active.
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
	}
	updateUnits.Updates = []*state.UpdateUnitOperation{
		noContainerUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("never-cloud-container"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			UnitStatus: &status.StatusInfo{
				Status:  status.Active,
				Message: "unit active",
			},
		}),
		existingUnit.UpdateOperation(state.UnitUpdateProperties{
			ProviderId: strPtr("unit-uuid"),
			Address:    strPtr("192.168.1.2"),
			Ports:      &[]string{"443"},
			AgentStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing running",
			},
			CloudContainerStatus: &status.StatusInfo{
				Status:  status.Running,
				Message: "existing container running",
			},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	if !aliveApp {
		c.Assert(err, jc.Satisfies, state.IsNotAlive)
		return
	}
	c.Assert(err, jc.ErrorIsNil)

	units, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 4)

	unitsById := make(map[string]*state.Unit)
	containerInfoById := make(map[string]state.CloudContainer)
	for _, u := range units {
		c.Assert(u.ShouldBeAssigned(), jc.IsFalse)
		containerInfo, err := u.ContainerInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(containerInfo.Unit(), gc.Equals, u.Name())
		c.Assert(containerInfo.ProviderId(), gc.Not(gc.Equals), "")
		unitsById[containerInfo.ProviderId()] = u
		containerInfoById[containerInfo.ProviderId()] = containerInfo
	}
	u, ok := unitsById["unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	info, ok := containerInfoById["unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	c.Check(u.Name(), gc.Equals, existingUnit.Name())
	c.Check(info.Address(), gc.NotNil)
	c.Check(*info.Address(), gc.DeepEquals, network.NewScopedAddress("192.168.1.2", network.ScopeMachineLocal))
	c.Check(info.Ports(), jc.DeepEquals, []string{"443"})
	statusInfo, err := u.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "existing running")
	history, err := u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, gc.Equals, status.Running)
		c.Assert(history[1].Status, gc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, gc.Equals, status.Running)
		c.Assert(history[0].Status, gc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}
	statusInfo, err = u.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, gc.Equals, "waiting for container")
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "existing container running")
	unitHistory, err := u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Running)
	c.Assert(unitHistory[0].Message, gc.Equals, "existing container running")

	u, ok = unitsById["never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	info, ok = containerInfoById["never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, gc.Equals, status.MessageWaitForContainer)

	u, ok = unitsById["add-never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	info, ok = containerInfoById["add-never-cloud-container"]
	c.Assert(ok, jc.IsTrue)
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Waiting)
	c.Assert(unitHistory[0].Message, gc.Equals, status.MessageWaitForContainer)

	u, ok = unitsById["new-unit-uuid"]
	info, ok = containerInfoById["new-unit-uuid"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(u.Name(), gc.Equals, "gitlab/3")
	c.Check(info.Address(), gc.NotNil)
	c.Check(*info.Address(), gc.DeepEquals, network.NewScopedAddress("192.168.1.1", network.ScopeMachineLocal))
	c.Assert(info.Ports(), jc.DeepEquals, []string{"80"})
	addr, err := u.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addr, jc.DeepEquals, network.Address{
		Value: "192.168.1.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeMachineLocal,
	})
	statusInfo, err = u.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, gc.Equals, status.MessageWaitForContainer)
	statusInfo, err = state.GetCloudContainerStatus(s.caasSt, u.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "new container running")
	statusInfo, err = u.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Running)
	c.Assert(statusInfo.Message, gc.Equals, "new running")
	history, err = u.AgentHistory().StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	// Creating a new unit may cause the history entries to be written with
	// the same timestamp due to the precision used by the db.
	if history[0].Status == status.Running {
		c.Assert(history[0].Status, gc.Equals, status.Running)
		c.Assert(history[1].Status, gc.Equals, status.Allocating)
	} else {
		c.Assert(history[1].Status, gc.Equals, status.Running)
		c.Assert(history[0].Status, gc.Equals, status.Allocating)
		c.Assert(history[0].Since.Unix(), gc.Equals, history[1].Since.Unix())
	}
	// container status history must have overridden the unit status.
	unitHistory, err = u.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitHistory[0].Status, gc.Equals, status.Running)
	c.Assert(unitHistory[0].Message, gc.Equals, "new container running")

	// check cloud container status history is stored.
	containerStatusHistory, err := state.GetCloudContainerStatusHistory(s.caasSt, u.Name(), status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(containerStatusHistory, gc.HasLen, 1)
	c.Assert(containerStatusHistory[0].Status, gc.Equals, status.Running)
	c.Assert(containerStatusHistory[0].Message, gc.Equals, "new container running")

	err = removedUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestAddUnitWithProviderId(c *gc.C) {
	u, err := s.app.AddUnit(state.AddUnitParams{ProviderId: strPtr("provider-id")})
	c.Assert(err, jc.ErrorIsNil)
	info, err := u.ContainerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Unit(), gc.Equals, u.Name())
	c.Assert(info.ProviderId(), gc.Equals, "provider-id")
}

func (s *CAASApplicationSuite) TestServiceInfo(c *gc.C) {
	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("id", []network.Address{{Value: "10.0.0.1"}})
		c.Assert(err, jc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, jc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.ProviderId(), gc.Equals, "id")
		c.Assert(info.Addresses(), jc.DeepEquals, []network.Address{{Value: "10.0.0.1"}})
	}
}

func (s *CAASApplicationSuite) TestServiceInfoEmptyProviderId(c *gc.C) {
	for i := 0; i < 2; i++ {
		err := s.app.UpdateCloudService("", []network.Address{{Value: "10.0.0.1"}})
		c.Assert(err, jc.ErrorIsNil)
		app, err := s.caasSt.Application(s.app.Name())
		c.Assert(err, jc.ErrorIsNil)
		info, err := app.ServiceInfo()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info.ProviderId(), gc.Equals, "")
		c.Assert(info.Addresses(), jc.DeepEquals, []network.Address{{Value: "10.0.0.1"}})
	}
}

func (s *CAASApplicationSuite) TestRemoveUnitDeletesServiceInfo(c *gc.C) {
	err := s.app.UpdateCloudService("id", []network.Address{{Value: "10.0.0.1"}})
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.app.ServiceInfo()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CAASApplicationSuite) TestInvalidScale(c *gc.C) {
	err := s.app.Scale(-1)
	c.Assert(err, gc.ErrorMatches, "application scale -1 not valid")
}

func (s *CAASApplicationSuite) TestScale(c *gc.C) {
	err := s.app.Scale(5)
	c.Assert(err, jc.ErrorIsNil)
	err = s.app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.app.GetScale(), gc.Equals, 5)
}

func (s *CAASApplicationSuite) TestWatchScale(c *gc.C) {
	// Empty initial event.
	w := s.app.WatchScale()
	defer testing.AssertStop(c, w)
	wc := testing.NewNotifyWatcherC(c, s.State, w)
	wc.AssertOneChange()

	err := s.app.Scale(5)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set to same value, no change.
	err = s.app.Scale(5)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.Scale(6)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// An unrelated update, no change.
	err = s.app.SetMinUnits(2)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *CAASApplicationSuite) TestRewriteStatusHistory(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS, CloudRegion: "<none>",
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	history, err := app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 1)
	c.Assert(history[0].Status, gc.Equals, status.Waiting)
	c.Assert(history[0].Message, gc.Equals, "waiting for container")

	// Must overwrite the history
	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Allocating,
		Message: "operator message",
	})
	c.Assert(err, jc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Status, gc.Equals, status.Allocating)
	c.Assert(history[0].Message, gc.Equals, "operator message")
	c.Assert(history[1].Status, gc.Equals, status.Waiting)
	c.Assert(history[1].Message, gc.Equals, "waiting for container")

	err = app.SetOperatorStatus(status.StatusInfo{
		Status:  status.Running,
		Message: "operator running",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = app.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "app active",
	})
	c.Assert(err, jc.ErrorIsNil)
	history, err = app.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Log(history)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Status, gc.Equals, status.Active)
	c.Assert(history[0].Message, gc.Equals, "app active")
	c.Assert(history[1].Status, gc.Equals, status.Allocating)
	c.Assert(history[1].Message, gc.Equals, "operator message")
	c.Assert(history[2].Status, gc.Equals, status.Waiting)
	c.Assert(history[2].Message, gc.Equals, "waiting for container")
}

func (s *ApplicationSuite) TestApplicationSetAgentPresence(c *gc.C) {
	alive, err := s.mysql.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	pinger, err := s.mysql.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pinger, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(pinger), jc.ErrorIsNil)
	}()
	s.State.StartSync()
	alive, err = s.mysql.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)
}

func (s *ApplicationSuite) TestApplicationWaitAgentPresence(c *gc.C) {
	alive, err := s.mysql.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)

	err = s.mysql.WaitAgentPresence(coretesting.ShortWait)
	c.Assert(err, gc.ErrorMatches, `waiting for agent of application "mysql": still not alive after timeout`)

	pinger, err := s.mysql.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	err = s.mysql.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)

	alive, err = s.mysql.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsTrue)

	err = pinger.KillForTesting()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()

	alive, err = s.mysql.AgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alive, jc.IsFalse)
}

func (s *ApplicationSuite) TestSetOperatorStatusNonCAAS(c *gc.C) {
	_, err := state.ApplicationOperatorStatus(s.State, s.mysql.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ApplicationSuite) TestSetOperatorStatus(c *gc.C) {
	st := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS, CloudRegion: "<none>",
	})
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	// Initial status.
	appStatus, err := state.ApplicationOperatorStatus(st, app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.DeepEquals, status.Waiting)
	c.Assert(appStatus.Message, gc.DeepEquals, "waiting for container")

	now := coretesting.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "broken",
		Since:   &now,
	}
	err = app.SetOperatorStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	appStatus, err = state.ApplicationOperatorStatus(st, app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.DeepEquals, status.Error)
	c.Assert(appStatus.Message, gc.DeepEquals, "broken")
}
