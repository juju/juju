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
	jc "launchpad.net/juju-core/testing/checkers"
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

func (s *clientSuite) TestCompatibleSettingsParsing(c *gc.C) {
	// Test the exported settings parsing in a compatible way.
	_, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	service, err := s.State.Service("dummy")
	c.Assert(err, gc.IsNil)
	ch, _, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(ch.URL().String(), gc.Equals, "local:quantal/dummy-1")

	// Empty string will be returned as nil.
	options := map[string]string{
		"title":    "foobar",
		"username": "",
	}
	settings, err := client.ParseSettingsCompatible(ch, options)
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": nil,
	})

	// Illegal settings lead to an error.
	options = map[string]string{
		"yummy": "didgeridoo",
	}
	settings, err = client.ParseSettingsCompatible(ch, options)
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

func (s *clientSuite) TestClientServiceSet(c *gc.C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "foobar",
		"username": "user name",
	})
	c.Assert(err, gc.IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "barfoo",
		"username": "",
	})
	c.Assert(err, gc.IsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "barfoo",
		"username": "",
	})
}

func (s *clientSuite) TestClientServerUnset(c *gc.C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "foobar",
		"username": "user name",
	})
	c.Assert(err, gc.IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.APIState.Client().ServiceUnset("dummy", []string{"username"})
	c.Assert(err, gc.IsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "foobar",
	})
}

func (s *clientSuite) TestClientServiceSetYAML(c *gc.C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	err = s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: foobar\n  username: user name\n")
	c.Assert(err, gc.IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	})

	err = s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: barfoo\n  username: \n")
	c.Assert(err, gc.IsNil)
	settings, err = dummy.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title": "barfoo",
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
		url:   "local:quantal/wordpress-3",
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
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
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

var serviceExposeTests = []struct {
	about   string
	service string
	err     string
	exposed bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "expose a service",
		service: "dummy-service",
		exposed: true,
	},
	{
		about:   "expose an already exposed service",
		service: "exposed-service",
		exposed: true,
	},
}

func (s *clientSuite) TestClientServiceExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	serviceNames := []string{"dummy-service", "exposed-service"}
	svcs := make([]*state.Service, len(serviceNames))
	var err error
	for i, name := range serviceNames {
		svcs[i], err = s.State.AddService(name, charm)
		c.Assert(err, gc.IsNil)
		c.Assert(svcs[i].IsExposed(), gc.Equals, false)
	}
	err = svcs[1].SetExposed()
	c.Assert(err, gc.IsNil)
	c.Assert(svcs[1].IsExposed(), gc.Equals, true)
	for i, t := range serviceExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err = s.APIState.Client().ServiceExpose(t.service)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
			service, err := s.State.Service(t.service)
			c.Assert(err, gc.IsNil)
			c.Assert(service.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

var serviceUnexposeTests = []struct {
	about    string
	service  string
	err      string
	initial  bool
	expected bool
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:    "unexpose a service",
		service:  "dummy-service",
		initial:  true,
		expected: false,
	},
	{
		about:    "unexpose an already unexposed service",
		service:  "dummy-service",
		initial:  false,
		expected: false,
	},
}

func (s *clientSuite) TestClientServiceUnexpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		svc, err := s.State.AddService("dummy-service", charm)
		c.Assert(err, gc.IsNil)
		if t.initial {
			svc.SetExposed()
		}
		c.Assert(svc.IsExposed(), gc.Equals, t.initial)
		err = s.APIState.Client().ServiceUnexpose(t.service)
		if t.err == "" {
			c.Assert(err, gc.IsNil)
			svc.Refresh()
			c.Assert(svc.IsExposed(), gc.Equals, t.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		err = svc.Destroy()
		c.Assert(err, gc.IsNil)
	}
}

var serviceDestroyTests = []struct {
	about   string
	service string
	err     string
}{
	{
		about:   "unknown service name",
		service: "unknown-service",
		err:     `service "unknown-service" not found`,
	},
	{
		about:   "destroy a service",
		service: "dummy-service",
	},
	{
		about:   "destroy an already destroyed service",
		service: "dummy-service",
		err:     `service "dummy-service" not found`,
	},
}

func (s *clientSuite) TestClientServiceDestroy(c *gc.C) {
	_, err := s.State.AddService("dummy-service", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)
	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err = s.APIState.Client().ServiceDestroy(t.service)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
	}

	// Now do ServiceDestroy on a service with units. Destroy will
	// cause the service to be not-Alive, but will not remove its
	// document.
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().ServiceDestroy(serviceName)
	c.Assert(err, gc.IsNil)
	err = service.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(service.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *clientSuite) TestClientDestroyMachines(c *gc.C) {
	s.setUpScenario(c)
	err := s.APIState.Client().DestroyMachines("1")
	c.Assert(err, gc.ErrorMatches, `no machines were destroyed: machine 1 has unit "wordpress/0" assigned`)
	m, err := s.State.AddMachine("trusty", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().DestroyMachines(m.Id())
	c.Assert(err, gc.IsNil)
	err = m.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(m.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *clientSuite) TestClientDestroyUnits(c *gc.C) {
	// Setup:
	s.setUpScenario(c)
	service, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	_, err = service.AddUnit()
	c.Assert(err, gc.IsNil)
	// Destroy some units and check the result.
	err = s.APIState.Client().DestroyServiceUnits([]string{"wordpress/1", "wordpress/2"})
	c.Assert(err, gc.IsNil)
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	for i, u := range units {
		if i == 0 {
			c.Assert(u.Life(), gc.Equals, state.Alive)
		} else {
			c.Assert(u.Life(), gc.Equals, state.Dying)
		}
	}
}

func (s *clientSuite) testClientUnitResolved(c *gc.C, retry bool, expectedResolvedMode state.ResolvedMode) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	err = u.SetStatus(params.StatusError, "gaaah", nil)
	c.Assert(err, gc.IsNil)
	// Code under test:
	err = s.APIState.Client().Resolved("wordpress/0", retry)
	c.Assert(err, gc.IsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, gc.IsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, gc.Equals, expectedResolvedMode)
}

func (s *clientSuite) TestClientUnitResolved(c *gc.C) {
	s.testClientUnitResolved(c, false, state.ResolvedNoHooks)
}

func (s *clientSuite) TestClientUnitResolvedRetry(c *gc.C) {
	s.testClientUnitResolved(c, true, state.ResolvedRetryHooks)
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
		c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
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

func (s *clientSuite) checkEndpoints(c *gc.C, endpoints map[string]charm.Relation) {
	c.Assert(endpoints["wordpress"], gc.DeepEquals, charm.Relation{
		Name:      "db",
		Role:      charm.RelationRole("requirer"),
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     charm.RelationScope("global"),
	})
	c.Assert(endpoints["mysql"], gc.DeepEquals, charm.Relation{
		Name:      "server",
		Role:      charm.RelationRole("provider"),
		Interface: "mysql",
		Optional:  false,
		Limit:     0,
		Scope:     charm.RelationScope("global"),
	})
}

func (s *clientSuite) assertAddRelation(c *gc.C, endpoints []string) {
	s.setUpScenario(c)
	res, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.IsNil)
	s.checkEndpoints(c, res.Endpoints)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	rels, err := wpSvc.Relations()
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), gc.Equals, 2)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *clientSuite) TestSuccessfullyAddRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints)
}

func (s *clientSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints)
}

func (s *clientSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress"}
	_, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *clientSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	s.setUpScenario(c)
	// Add a relation between wordpress and mysql.
	endpoints := []string{"wordpress", "mysql"}
	eps, err := s.State.InferEndpoints(endpoints)
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	// And try to add it again.
	_, err = s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}

func (s *clientSuite) assertDestroyRelation(c *gc.C, endpoints []string) {
	s.setUpScenario(c)
	// Add a relation between the endpoints.
	eps, err := s.State.InferEndpoints(endpoints)
	c.Assert(err, gc.IsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.IsNil)
	// Show that the relation was removed.
	c.Assert(relation.Refresh(), jc.Satisfies, errors.IsNotFoundError)
}

func (s *clientSuite) TestSuccessfulDestroyRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *clientSuite) TestSuccessfullyDestroyRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the DestroyRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertDestroyRelation(c, endpoints)
}

func (s *clientSuite) TestNoRelation(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestAttemptDestroyingNonExistentRelation(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, gc.IsNil)
	endpoints := []string{"riak", "wordpress"}
	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestAttemptDestroyingWithOnlyOneEndpoint(c *gc.C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *clientSuite) TestAttemptDestroyingPeerRelation(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, gc.IsNil)

	endpoints := []string{"riak:ring"}
	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
}

func (s *clientSuite) TestAttemptDestroyingAlreadyDestroyedRelation(c *gc.C) {
	s.setUpScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	endpoints := []string{"wordpress", "mysql"}
	err = s.APIState.Client().DestroyRelation(endpoints...)
	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFoundError)

	// And try to destroy it again.
	err = s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestClientWatchAll(c *gc.C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("quantal", state.JobManageEnviron)
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

func (s *clientSuite) TestClientSetServiceConstraints(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Update constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().SetServiceConstraints("dummy", cons)
	c.Assert(err, gc.IsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientGetServiceConstraints(c *gc.C) {
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, gc.IsNil)

	// Set constraints for the service.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	err = service.SetConstraints(cons)
	c.Assert(err, gc.IsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetServiceConstraints("dummy")
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientSetEnvironmentConstraints(c *gc.C) {
	// Set constraints for the environment.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().SetEnvironmentConstraints(cons)
	c.Assert(err, gc.IsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := s.State.EnvironConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientGetEnvironmentConstraints(c *gc.C) {
	// Set constraints for the environment.
	cons, err := constraints.Parse("mem=4096", "cpu-cores=2")
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConstraints(cons)
	c.Assert(err, gc.IsNil)

	// Check we can get the constraints.
	obtained, err := s.APIState.Client().GetEnvironmentConstraints()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *clientSuite) TestClientEnvironmentGet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs, err := s.APIState.Client().EnvironmentGet()
	c.Assert(err, gc.IsNil)
	allAttrs := envConfig.AllAttrs()
	// We cannot simply use DeepEquals, because after the
	// map[string]interface{} result of EnvironmentGet is
	// serialized to JSON, integers are converted to floats.
	for key, apiValue := range attrs {
		envValue, found := allAttrs[key]
		c.Check(found, jc.IsTrue)
		switch apiValue.(type) {
		case float64, float32:
			c.Check(fmt.Sprintf("%v", envValue), gc.Equals, fmt.Sprintf("%v", apiValue))
		default:
			c.Check(envValue, gc.Equals, apiValue)
		}
	}
}

func (s *clientSuite) TestClientEnvironmentSet(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	_, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsFalse)

	args := map[string]interface{}{"some-key": "value"}
	err = s.APIState.Client().EnvironmentSet(args)
	c.Assert(err, gc.IsNil)

	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	value, found := envConfig.AllAttrs()["some-key"]
	c.Assert(found, jc.IsTrue)
	c.Assert(value, gc.Equals, "value")
}
