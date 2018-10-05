// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/constraints"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

// DeployLocalSuite uses a fresh copy of the same local dummy charm for each
// test, because DeployApplication demands that a charm already exists in state,
// and that's is the simplest way to get one in there.
type DeployLocalSuite struct {
	testing.JujuConnSuite
	repo  charmrepo.Interface
	charm *state.Charm
}

var _ = gc.Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.repo = &charmrepo.LocalRepository{Path: testcharms.Repo.Path()}
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
}

func (s *DeployLocalSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	curl := charm.MustParseURL("local:quantal/dummy")
	charm, err := testing.PutCharm(s.State, curl, s.repo, false, false)
	c.Assert(err, jc.ErrorIsNil)
	s.charm = charm
}

func (s *DeployLocalSuite) TestDeployMinimal(c *gc.C) {
	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertCharm(c, app, s.charm.URL())
	s.assertSettings(c, app, charm.Settings{})
	s.assertApplicationConfig(c, app, coreapplication.ConfigAttributes(nil))
	s.assertConstraints(c, app, constraints.Value{})
	s.assertMachines(c, app, constraints.Value{})
}

func (s *DeployLocalSuite) TestDeploySeries(c *gc.C) {
	var f fakeDeployer

	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Series:          "aseries",
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Series, gc.Equals, "aseries")
}

func (s *DeployLocalSuite) TestDeployWithImplicitBindings(c *gc.C) {
	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)

	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName:  "bob",
			Charm:            wordpressCharm,
			EndpointBindings: nil,
		})
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		// relation names
		"url":             "",
		"logging-dir":     "",
		"monitoring-port": "",
		"db":              "",
		"cache":           "",
		"cluster":         "",
		// extra-bindings names
		"db-client": "",
		"admin-api": "",
		"foo-bar":   "",
	})
}

func (s *DeployLocalSuite) addWordpressCharm(c *gc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmWithExtraBindings(c *gc.C) *state.Charm {
	wordpressCharmURL := charm.MustParseURL("local:quantal/wordpress-extra-bindings")
	return s.addWordpressCharmFromURL(c, wordpressCharmURL)
}

func (s *DeployLocalSuite) addWordpressCharmFromURL(c *gc.C, charmURL *charm.URL) *state.Charm {
	wordpressCharm, err := testing.PutCharm(s.State, charmURL, s.repo, false, false)
	c.Assert(err, jc.ErrorIsNil)
	return wordpressCharm
}

func (s *DeployLocalSuite) assertBindings(c *gc.C, app application.Application, expected map[string]string) {
	type withEndpointBindings interface {
		EndpointBindings() (map[string]string, error)
	}
	bindings, err := app.(withEndpointBindings).EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bindings, jc.DeepEquals, expected)
}

func (s *DeployLocalSuite) TestDeployWithSomeSpecifiedBindings(c *gc.C) {
	wordpressCharm := s.addWordpressCharm(c)
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   "public",
				"db": "db",
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		// default binding
		"": "public",
		// relation names
		"url":             "public",
		"logging-dir":     "public",
		"monitoring-port": "public",
		"db":              "db",
		"cache":           "public",
		// extra-bindings names
		"db-client": "public",
		"admin-api": "public",
		"foo-bar":   "public",
	})
}

func (s *DeployLocalSuite) TestDeployWithBoundRelationNamesAndExtraBindingsNames(c *gc.C) {
	wordpressCharm := s.addWordpressCharmWithExtraBindings(c)
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("internal", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":          "public",
				"db":        "db",
				"db-client": "db",
				"admin-api": "internal",
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	s.assertBindings(c, app, map[string]string{
		"":                "public",
		"url":             "public",
		"logging-dir":     "public",
		"monitoring-port": "public",
		"db":              "db",
		"cache":           "public",
		"db-client":       "db",
		"admin-api":       "internal",
		"cluster":         "public",
		"foo-bar":         "public", // like for relations, uses the application-default.
	})

}

func (s *DeployLocalSuite) TestDeployWithInvalidSpace(c *gc.C) {
	wordpressCharm := s.addWordpressCharm(c)
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("internal", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":   "public",
				"db": "intranel", //typo 'intranel'
			},
		})
	c.Assert(err, gc.ErrorMatches, `cannot add application "bob": unknown space "intranel" not valid`)
	c.Check(app, gc.IsNil)
	// The application should not have been added
	_, err = s.State.Application("bob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

}

func (s *DeployLocalSuite) TestDeployWithInvalidBinding(c *gc.C) {
	wordpressCharm := s.addWordpressCharm(c)
	_, err := s.State.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("public", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("internal", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           wordpressCharm,
			EndpointBindings: map[string]string{
				"":      "public",
				"bd":    "internal", // typo 'bd'
				"pubic": "public",   // typo 'pubic'
			},
		})
	c.Assert(err, gc.ErrorMatches,
		`invalid binding\(s\) supplied "bd", "pubic", valid binding names are "admin-api", .* "url"`)
	c.Check(app, gc.IsNil)
	// The application should not have been added
	_, err = s.State.Application("bob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

}

func (s *DeployLocalSuite) TestDeployResources(c *gc.C) {
	var f fakeDeployer

	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			EndpointBindings: map[string]string{
				"": "public",
			},
			Resources: map[string]string{"foo": "bar"},
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Resources, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *DeployLocalSuite) TestDeploySettings(c *gc.C) {
	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"title":       "banana cupcakes",
				"skill-level": 9901,
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSettings(c, app, charm.Settings{
		"title":       "banana cupcakes",
		"skill-level": int64(9901),
	})
}

func (s *DeployLocalSuite) TestDeploySettingsError(c *gc.C) {
	_, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			CharmConfig: charm.Settings{
				"skill-level": 99.01,
			},
		})
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got 99.01`)
	_, err = s.State.Application("bob")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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

func (s *DeployLocalSuite) TestDeployWithApplicationConfig(c *gc.C) {
	cfg, err := coreapplication.NewConfig(map[string]interface{}{
		"outlook":     "good",
		"skill-level": 1,
	}, sampleApplicationConfigSchema(), nil)
	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName:   "bob",
			Charm:             s.charm,
			ApplicationConfig: cfg,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertApplicationConfig(c, app, coreapplication.ConfigAttributes{
		"outlook":     "good",
		"skill-level": 1,
	})
}

func (s *DeployLocalSuite) TestDeployConstraints(c *gc.C) {
	err := s.State.SetModelConstraints(constraints.MustParse("mem=2G"))
	c.Assert(err, jc.ErrorIsNil)
	applicationCons := constraints.MustParse("cores=2")
	app, err := application.DeployApplication(stateDeployer{s.State},
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.assertConstraints(c, app, applicationCons)
}

func (s *DeployLocalSuite) TestDeployNumUnits(c *gc.C) {
	var f fakeDeployer

	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        2,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 2)
}

func (s *DeployLocalSuite) TestDeployForceMachineId(c *gc.C) {
	var f fakeDeployer

	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("0")},
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 1)
	c.Assert(f.args.Placement, gc.HasLen, 1)
	c.Assert(*f.args.Placement[0], gc.Equals, instance.Placement{Scope: instance.MachineScope, Directive: "0"})
}

func (s *DeployLocalSuite) TestDeployForceMachineIdWithContainer(c *gc.C) {
	var f fakeDeployer

	applicationCons := constraints.MustParse("cores=2")
	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement(fmt.Sprintf("%s:0", instance.LXD))},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 1)
	c.Assert(f.args.Placement, gc.HasLen, 1)
	c.Assert(*f.args.Placement[0], gc.Equals, instance.Placement{Scope: string(instance.LXD), Directive: "0"})
}

func (s *DeployLocalSuite) TestDeploy(c *gc.C) {
	var f fakeDeployer

	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{
		{Scope: s.State.ModelUUID(), Directive: "valid"},
		{Scope: "#", Directive: "0"},
		{Scope: "lxd", Directive: "1"},
		{Scope: "lxd", Directive: ""},
	}
	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        4,
			Placement:       placement,
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 4)
	c.Assert(f.args.Placement, gc.DeepEquals, placement)
}

func (s *DeployLocalSuite) TestDeployWithFewerPlacement(c *gc.C) {
	var f fakeDeployer
	applicationCons := constraints.MustParse("cores=2")
	placement := []*instance.Placement{{Scope: s.State.ModelUUID(), Directive: "valid"}}
	_, err := application.DeployApplication(&f,
		application.DeployApplicationParams{
			ApplicationName: "bob",
			Charm:           s.charm,
			Constraints:     applicationCons,
			NumUnits:        3,
			Placement:       placement,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.args.Name, gc.Equals, "bob")
	c.Assert(f.args.Charm, gc.DeepEquals, s.charm)
	c.Assert(f.args.Constraints, gc.DeepEquals, applicationCons)
	c.Assert(f.args.NumUnits, gc.Equals, 3)
	c.Assert(f.args.Placement, gc.DeepEquals, placement)
}

func (s *DeployLocalSuite) assertCharm(c *gc.C, app application.Application, expect *charm.URL) {
	curl, force := app.CharmURL()
	c.Assert(curl, gc.DeepEquals, expect)
	c.Assert(force, jc.IsFalse)
}

func (s *DeployLocalSuite) assertSettings(c *gc.C, app application.Application, settings charm.Settings) {
	settings, err := app.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected := s.charm.Config().DefaultSettings()
	for name, value := range settings {
		expected[name] = value
	}
	c.Assert(settings, gc.DeepEquals, expected)
}

func (s *DeployLocalSuite) assertApplicationConfig(c *gc.C, app application.Application, wantCfg coreapplication.ConfigAttributes) {
	cfg, err := app.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.DeepEquals, wantCfg)
}

func (s *DeployLocalSuite) assertConstraints(c *gc.C, app application.Application, expect constraints.Value) {
	cons, err := app.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, expect)
}

func (s *DeployLocalSuite) assertMachines(c *gc.C, app application.Application, expectCons constraints.Value, expectIds ...string) {
	type withAssignedMachineId interface {
		AssignedMachineId() (string, error)
	}

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	// first manually tell state to assign all the units
	for _, unit := range units {
		id := unit.UnitTag().Id()
		res, err := s.State.AssignStagedUnits([]string{id})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(res[0].Error, jc.ErrorIsNil)
		c.Assert(res[0].Unit, gc.Equals, id)
	}

	// refresh the list of units from state
	units, err = app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, len(expectIds))
	unseenIds := set.NewStrings(expectIds...)
	for _, unit := range units {
		id, err := unit.(withAssignedMachineId).AssignedMachineId()
		c.Assert(err, jc.ErrorIsNil)
		unseenIds.Remove(id)
		machine, err := s.State.Machine(id)
		c.Assert(err, jc.ErrorIsNil)
		cons, err := machine.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cons, gc.DeepEquals, expectCons)
	}
	c.Assert(unseenIds, gc.DeepEquals, set.NewStrings())
}

type stateDeployer struct {
	*state.State
}

func (d stateDeployer) AddApplication(args state.AddApplicationArgs) (application.Application, error) {
	app, err := d.State.AddApplication(args)
	if err != nil {
		return nil, err
	}
	return application.NewStateApplication(d.State, app), nil
}

type fakeDeployer struct {
	args state.AddApplicationArgs
}

func (f *fakeDeployer) AddApplication(args state.AddApplicationArgs) (application.Application, error) {
	f.args = args
	return nil, nil
}
