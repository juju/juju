// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/client"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type clientSuite struct {
	baseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestClientStatus(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientServerSet(c *gc.C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "xxx",
		"username": "yyy",
	})
	c.Assert(err, gc.IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "xxx",
		"username": "yyy",
	})
}

func (s *clientSuite) TestClientServiceSetYAML(c *gc.C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: aaa\n  username: bbb")
	c.Assert(err, gc.IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "aaa",
		"username": "bbb",
	})
}

var clientAddServiceUnitsTests = []struct {
	about    string
	service  string // if not set, defaults to 'dummy'
	expected []string
	to       string
	err      string
}{
	{
		about:    "returns unit names",
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		about:    "cannot mix to when adding multiple units",
		err:      "cannot use NumUnits with ToMachineSpec",
		expected: []string{"dummy/0", "dummy/1"},
		to:       "0",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
}

func (s *clientSuite) TestClientAddServiceUnits(c *gc.C) {
	_, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	for i, t := range clientAddServiceUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		serviceName := t.service
		if serviceName == "" {
			serviceName = "dummy"
		}
		units, err := s.APIState.Client().AddServiceUnits(serviceName, len(t.expected), t.to)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(units, gc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/3")
	c.Assert(err, gc.IsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

var clientCharmInfoTests = []struct {
	about string
	url   string
	err   string
}{
	{
		about: "retrieves charm info",
		url:   "local:series/wordpress-3",
	},
	{
		about: "invalid URL",
		url:   "not-valid",
		err:   `charm URL has invalid schema: "not-valid"`,
	},
	{
		about: "unknown charm",
		url:   "cs:missing/one-1",
		err:   `charm "cs:missing/one-1" not found`,
	},
}

func (s *clientSuite) TestClientCharmInfo(c *gc.C) {
	// Use wordpress for tests so that we can compare Provides and Requires.
	charm := s.AddTestingCharm(c, "wordpress")
	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)
		info, err := s.APIState.Client().CharmInfo(t.url)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		expected := &api.CharmInfo{
			Revision: charm.Revision(),
			URL:      charm.URL().String(),
			Config:   charm.Config(),
			Meta:     charm.Meta(),
		}
		c.Assert(info, gc.DeepEquals, expected)
	}
}

func (s *clientSuite) TestClientEnvironmentInfo(c *gc.C) {
	conf, _ := s.State.EnvironConfig()
	info, err := s.APIState.Client().EnvironmentInfo()
	c.Assert(err, gc.IsNil)
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(info.DefaultSeries, gc.Equals, conf.DefaultSeries())
	c.Assert(info.ProviderType, gc.Equals, conf.Type())
	c.Assert(info.Name, gc.Equals, conf.Name())
	c.Assert(info.UUID, gc.Equals, env.UUID())
}

var clientAnnotationsTests = []struct {
	about    string
	initial  map[string]string
	input    map[string]string
	expected map[string]string
	err      string
}{
	{
		about:    "test setting an annotation",
		input:    map[string]string{"mykey": "myvalue"},
		expected: map[string]string{"mykey": "myvalue"},
	},
	{
		about:    "test setting multiple annotations",
		input:    map[string]string{"key1": "value1", "key2": "value2"},
		expected: map[string]string{"key1": "value1", "key2": "value2"},
	},
	{
		about:    "test overriding annotations",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": "another-value"},
		expected: map[string]string{"mykey": "another-value"},
	},
	{
		about: "test setting an invalid annotation",
		input: map[string]string{"invalid.key": "myvalue"},
		err:   `cannot update annotations on .*: invalid key "invalid.key"`,
	},
}

func (s *clientSuite) TestClientAnnotations(c *gc.C) {
	// Set up entities.
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	environment, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	type taggedAnnotator interface {
		state.Annotator
		state.Entity
	}
	entities := []taggedAnnotator{service, unit, machine, environment}
	for i, t := range clientAnnotationsTests {
		for _, entity := range entities {
			id := entity.Tag()
			c.Logf("test %d. %s. entity %s", i, t.about, id)
			// Set initial entity annotations.
			err := entity.SetAnnotations(t.initial)
			c.Assert(err, gc.IsNil)
			// Add annotations using the API call.
			err = s.APIState.Client().SetAnnotations(id, t.input)
			if t.err != "" {
				c.Assert(err, gc.ErrorMatches, t.err)
				continue
			}
			// Check annotations are correctly set.
			dbann, err := entity.Annotations()
			c.Assert(err, gc.IsNil)
			c.Assert(dbann, gc.DeepEquals, t.expected)
			// Retrieve annotations using the API call.
			ann, err := s.APIState.Client().GetAnnotations(id)
			c.Assert(err, gc.IsNil)
			// Check annotations are correctly returned.
			c.Assert(ann, gc.DeepEquals, dbann)
			// Clean up annotations on the current entity.
			cleanup := make(map[string]string)
			for key := range dbann {
				cleanup[key] = ""
			}
			err = entity.SetAnnotations(cleanup)
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s *clientSuite) TestClientAnnotationsBadEntity(c *gc.C) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo", "service-", "service-foo/bar"}
	expected := `".*" is not a valid( [a-z]+)? tag`
	for _, id := range bad {
		err := s.APIState.Client().SetAnnotations(id, map[string]string{"mykey": "myvalue"})
		c.Assert(err, gc.ErrorMatches, expected)
		_, err = s.APIState.Client().GetAnnotations(id)
		c.Assert(err, gc.ErrorMatches, expected)
	}
}

func (s *clientSuite) TestClientServiceGet(c *gc.C) {
	s.setUpScenario(c)
	results, err := s.APIState.Client().ServiceGet("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, &params.ServiceGetResults{
		Service: "wordpress",
		Charm:   "wordpress",
		Config: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"type":        "string",
				"value":       "My Title",
				"description": "A descriptive title used for the blog.",
				"default":     true,
			},
		},
	})
}

func (s *clientSuite) TestClientServiceExpose(c *gc.C) {
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	c.Assert(service.IsExposed(), gc.Equals, false)
	err = s.APIState.Client().ServiceExpose(serviceName)
	c.Assert(err, gc.IsNil)
	err = service.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(service.IsExposed(), gc.Equals, true)
}

func (s *clientSuite) TestClientServiceUnexpose(c *gc.C) {
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	service.SetExposed()
	c.Assert(service.IsExposed(), gc.Equals, true)
	err = s.APIState.Client().ServiceUnexpose(serviceName)
	c.Assert(err, gc.IsNil)
	service.Refresh()
	c.Assert(service.IsExposed(), gc.Equals, false)
}

func (s *clientSuite) TestClientServiceDestroy(c *gc.C) {
	// Setup:
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	// Code under test:
	err = s.APIState.Client().ServiceDestroy(serviceName)
	c.Assert(err, gc.IsNil)
	err = service.Refresh()
	// The test actual assertion: the service should no-longer be Alive.
	c.Assert(service.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *clientSuite) TestClientUnitResolved(c *gc.C) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	err = u.SetStatus(params.StatusError, "gaaah")
	c.Assert(err, gc.IsNil)
	// Code under test:
	err = s.APIState.Client().Resolved("wordpress/0", false)
	c.Assert(err, gc.IsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, gc.IsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNoHooks)
}

func (s *clientSuite) TestClientServiceDeployCharmErrors(c *gc.C) {
	_, restore := makeMockCharmStore()
	defer restore()
	for url, expect := range map[string]string{
		// TODO(fwereade) make these errors consistent one day.
		"wordpress":                      `charm URL has invalid schema: "wordpress"`,
		"cs:wordpress":                   `charm URL without series: "cs:wordpress"`,
		"cs:precise/wordpress":           "charm url must include revision",
		"cs:precise/wordpress-999999":    `cannot get charm: charm not found in mock store: cs:precise/wordpress-999999`,
		"local:precise/wordpress-999999": `charm url has unsupported schema "local"`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceDeploy(
			url, "service", 1, "", constraints.Value{},
		)
		c.Check(err, gc.ErrorMatches, expect)
		_, err = s.State.Service("service")
		c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	}
}

func (s *clientSuite) TestClientServiceDeployPrincipal(c *gc.C) {
	// TODO(fwereade): test ToMachineSpec directly on srvClient, when we
	// manage to extract it as a package and can thus do it conveniently.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "dummy")
	mem4g := constraints.MustParse("mem=4G")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g,
	)
	c.Assert(err, gc.IsNil)
	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(force, gc.Equals, false)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	cons, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(cons, gc.DeepEquals, mem4g)
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, gc.IsNil)
		machine, err := s.State.Machine(mid)
		c.Assert(err, gc.IsNil)
		cons, err := machine.Constraints()
		c.Assert(err, gc.IsNil)
		c.Assert(cons, gc.DeepEquals, mem4g)
	}
}

func (s *clientSuite) TestClientServiceDeploySubordinate(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "logging")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 0, "", constraints.Value{},
	)
	service, err := s.State.Service("service-name")
	c.Assert(err, gc.IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(force, gc.Equals, false)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *clientSuite) TestClientServiceDeployConfig(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{},
	)
	c.Assert(err, gc.IsNil)
	service, err := s.State.Service("service-name")
	c.Assert(err, gc.IsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"username": "fred"})
}

func (s *clientSuite) TestClientServiceDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  skill-level: fred", constraints.Value{},
	)
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *clientSuite) deployServiceForTests(c *gc.C, store *coretesting.MockCharmStore) {
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(curl.String(),
		"service", 1, "", constraints.Value{},
	)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) checkClientServiceUpdateSetCharm(c *gc.C, forceCharmUrl bool) {
	store, restore := makeMockCharmStore()
	defer restore()
	s.deployServiceForTests(c, store)
	addCharm(c, store, "wordpress")

	// Update the charm for the service.
	args := params.ServiceUpdate{
		ServiceName:   "service",
		CharmUrl:      "cs:precise/wordpress-3",
		ForceCharmUrl: forceCharmUrl,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)
	ch, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, gc.Equals, forceCharmUrl)
}

func (s *clientSuite) TestClientServiceUpdateSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, false)
}

func (s *clientSuite) TestClientServiceUpdateForceSetCharm(c *gc.C) {
	s.checkClientServiceUpdateSetCharm(c, true)
}

func (s *clientSuite) TestClientServiceUpdateSetCharmErrors(c *gc.C) {
	_, restore := makeMockCharmStore()
	defer restore()
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	for charmUrl, expect := range map[string]string{
		// TODO(fwereade,Makyo) make these errors consistent one day.
		"wordpress":                      `charm URL has invalid schema: "wordpress"`,
		"cs:wordpress":                   `charm URL without series: "cs:wordpress"`,
		"cs:precise/wordpress":           "charm url must include revision",
		"cs:precise/wordpress-999999":    `cannot get charm: charm not found in mock store: cs:precise/wordpress-999999`,
		"local:precise/wordpress-999999": `charm url has unsupported schema "local"`,
	} {
		c.Logf("test %s", charmUrl)
		args := params.ServiceUpdate{
			ServiceName: "wordpress",
			CharmUrl:    charmUrl,
		}
		err := s.APIState.Client().ServiceUpdate(args)
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func (s *clientSuite) TestClientServiceUpdateSetMinUnits(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Set minimum units for the service.
	minUnits := 2
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, minUnits)
}

func (s *clientSuite) TestClientServiceUpdateSetMinUnitsError(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Set a negative minimum number of units for the service.
	minUnits := -1
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for service "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, 0)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsStrings(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:     "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsYAML(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "dummy:\n  title: y-title\n  username: y-user",
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetConstraints(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		Constraints: &cons,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientServiceUpdateAllParams(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	s.deployServiceForTests(c, store)
	addCharm(c, store, "wordpress")

	// Update all the service attributes.
	minUnits := 3
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	args := params.ServiceUpdate{
		ServiceName:     "service",
		CharmUrl:        "cs:precise/wordpress-3",
		ForceCharmUrl:   true,
		MinUnits:        &minUnits,
		SettingsStrings: map[string]string{"blog-title": "string-title"},
		SettingsYAML:    "service:\n  blog-title: yaml-title\n",
		Constraints:     &cons,
	}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the service has been correctly updated.
	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)

	// Check the charm.
	ch, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, gc.Equals, true)

	// Check the minimum number of units.
	c.Assert(service.MinUnits(), gc.Equals, minUnits)

	// Check the settings: also ensure the YAML settings take precedence
	// over strings ones.
	expectedSettings := charm.Settings{"blog-title": "yaml-title"}
	obtainedSettings, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(obtainedSettings, gc.DeepEquals, expectedSettings)

	// Check the constraints.
	obtainedConstraints, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(obtainedConstraints, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientServiceUpdateNoParams(c *gc.C) {
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)

	// Calling ServiceUpdate with no parameters set is a no-op.
	args := params.ServiceUpdate{ServiceName: "wordpress"}
	err = s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestClientServiceUpdateNoService(c *gc.C) {
	err := s.APIState.Client().ServiceUpdate(params.ServiceUpdate{})
	c.Assert(err, gc.ErrorMatches, `"" is not a valid service name`)
}

func (s *clientSuite) TestClientServiceUpdateInvalidService(c *gc.C) {
	args := params.ServiceUpdate{ServiceName: "no-such-service"}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches, `service "no-such-service" not found`)
}

func (s *clientSuite) TestClientServiceSetCharm(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{},
	)
	c.Assert(err, gc.IsNil)
	addCharm(c, store, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", false,
	)
	c.Assert(err, gc.IsNil)

	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, gc.Equals, false)
}

func (s *clientSuite) TestClientServiceSetCharmForce(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{},
	)
	c.Assert(err, gc.IsNil)
	addCharm(c, store, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, gc.IsNil)

	// Ensure that the charm is marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:precise/wordpress-3")
	c.Assert(force, gc.Equals, true)
}

func (s *clientSuite) TestClientServiceSetCharmInvalidService(c *gc.C) {
	_, restore := makeMockCharmStore()
	defer restore()
	err := s.APIState.Client().ServiceSetCharm(
		"badservice", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, gc.ErrorMatches, `service "badservice" not found`)
}

func (s *clientSuite) TestClientServiceSetCharmErrors(c *gc.C) {
	_, restore := makeMockCharmStore()
	defer restore()
	s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	for url, expect := range map[string]string{
		// TODO(fwereade,Makyo) make these errors consistent one day.
		"wordpress":                      `charm URL has invalid schema: "wordpress"`,
		"cs:wordpress":                   `charm URL without series: "cs:wordpress"`,
		"cs:precise/wordpress":           "charm url must include revision",
		"cs:precise/wordpress-999999":    `cannot get charm: charm not found in mock store: cs:precise/wordpress-999999`,
		"local:precise/wordpress-999999": `charm url has unsupported schema "local"`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceSetCharm(
			"wordpress", url, false,
		)
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func makeMockCharmStore() (store *coretesting.MockCharmStore, restore func()) {
	mockStore := coretesting.NewMockCharmStore()
	origStore := client.CharmStore
	client.CharmStore = mockStore
	return mockStore, func() { client.CharmStore = origStore }
}

func addCharm(c *gc.C, store *coretesting.MockCharmStore, name string) (*charm.URL, charm.Charm) {
	bundle := coretesting.Charms.Bundle(c.MkDir(), name)
	scurl := fmt.Sprintf("cs:precise/%s-%d", name, bundle.Revision())
	curl := charm.MustParseURL(scurl)
	err := store.SetCharm(curl, bundle)
	c.Assert(err, gc.IsNil)
	return curl, bundle
}

func (s *clientSuite) TestSuccessfulAddRelation(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql"}
	res, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.IsNil)
	c.Assert(res.Endpoints["wordpress"].Name, gc.Equals, "db")
	c.Assert(res.Endpoints["wordpress"].Interface, gc.Equals, "mysql")
	c.Assert(res.Endpoints["wordpress"].Scope, gc.Equals, charm.RelationScope("global"))
	c.Assert(res.Endpoints["mysql"].Name, gc.Equals, "server")
	c.Assert(res.Endpoints["mysql"].Interface, gc.Equals, "mysql")
	c.Assert(res.Endpoints["mysql"].Scope, gc.Equals, charm.RelationScope("global"))
	for _, endpoint := range endpoints {
		svc, err := s.State.Service(endpoint)
		c.Assert(err, gc.IsNil)
		rels, err := svc.Relations()
		c.Assert(err, gc.IsNil)
		for _, rel := range rels {
			c.Assert(rel.Life(), gc.Equals, state.Alive)
		}
	}
}

func (s *clientSuite) TestSuccessfulDestroyRelation(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "logging"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.IsNil)
	for _, endpoint := range endpoints {
		service, err := s.State.Service(endpoint)
		c.Assert(err, gc.IsNil)
		rels, err := service.Relations()
		c.Assert(err, gc.IsNil)
		// When relations are destroyed they don't go away immediately but
		// instead are set to 'Dying', due to references held by the user
		// agent.
		for _, rel := range rels {
			c.Assert(rel.Life(), gc.Equals, state.Dying)
		}
	}
}

func (s *clientSuite) TestNoRelation(c *gc.C) {
	s.setUpScenario(c)
	err := s.APIState.Client().DestroyRelation("wordpress", "mysql")
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestClientWatchAll(c *gc.C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = m.SetProvisioned("i-0", state.BootstrapNonce, nil)
	c.Assert(err, gc.IsNil)
	watcher, err := s.APIState.Client().WatchAll()
	c.Assert(err, gc.IsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, gc.IsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, gc.IsNil)
	if !c.Check(deltas, gc.DeepEquals, []params.Delta{{
		Entity: &params.MachineInfo{
			Id:         m.Id(),
			InstanceId: "i-0",
			Status:     params.StatusPending,
		},
	}}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}
