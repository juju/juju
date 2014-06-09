// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/charm"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual"
	envstorage "github.com/juju/juju/environs/storage"
	toolstesting "github.com/juju/juju/environs/tools/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/client"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type clientSuite struct {
	baseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestClientStatus(c *gc.C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestCompatibleSettingsParsing(c *gc.C) {
	// Test the exported settings parsing in a compatible way.
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
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
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSet("dummy", map[string]string{
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
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSet("dummy", map[string]string{
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
	dummy := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: foobar\n  username: user name\n")
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
	s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
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
		err:   "charm url series is not resolved",
	},
	{
		about: "invalid schema",
		url:   "not-valid:your-arguments",
		err:   `charm URL has invalid schema: "not-valid:your-arguments"`,
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
	c.Assert(info.DefaultSeries, gc.Equals, config.PreferredSeries(conf))
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
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))
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
		svcs[i] = s.AddTestingService(c, name, charm)
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
		svc := s.AddTestingService(c, "dummy-service", charm)
		if t.initial {
			svc.SetExposed()
		}
		c.Assert(svc.IsExposed(), gc.Equals, t.initial)
		err := s.APIState.Client().ServiceUnexpose(t.service)
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
	s.AddTestingService(c, "dummy-service", s.AddTestingCharm(c, "dummy"))
	for i, t := range serviceDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.APIState.Client().ServiceDestroy(t.service)
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

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func assertRemoved(c *gc.C, entity state.Living) {
	err := entity.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *clientSuite) setupDestroyMachinesTest(c *gc.C) (*state.Machine, *state.Machine, *state.Machine, *state.Unit) {
	m0, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	sch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingService(c, "wordpress", sch)
	u, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m1)
	c.Assert(err, gc.IsNil)

	return m0, m1, m2, u
}

func (s *clientSuite) TestDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)

	err := s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment; machine 1 has unit "wordpress/0" assigned`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Dying)

	err = u.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().DestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dying)
	assertLife(c, m2, state.Dying)
}

func (s *clientSuite) TestForceDestroyMachines(c *gc.C) {
	m0, m1, m2, u := s.setupDestroyMachinesTest(c)

	err := s.APIState.Client().ForceDestroyMachines("0", "1", "2")
	c.Assert(err, gc.ErrorMatches, `some machines were not destroyed: machine 0 is required by the environment`)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Alive)
	assertLife(c, m2, state.Alive)
	assertLife(c, u, state.Alive)

	err = s.State.Cleanup()
	c.Assert(err, gc.IsNil)
	assertLife(c, m0, state.Alive)
	assertLife(c, m1, state.Dead)
	assertLife(c, m2, state.Dead)
	assertRemoved(c, u)
}

func (s *clientSuite) TestDestroyPrincipalUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit()
		c.Assert(err, gc.IsNil)
		err = unit.SetStatus(params.StatusStarted, "", nil)
		c.Assert(err, gc.IsNil)
		units[i] = unit
	}

	// Destroy 2 of them; check they become Dying.
	err := s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/1")
	c.Assert(err, gc.IsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.APIState.Client().DestroyServiceUnits("wordpress/2", "wordpress/0")
	c.Assert(err, gc.IsNil)
	assertLife(c, units[2], state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.APIState.Client().DestroyServiceUnits("boojum/123", "wordpress/3")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	assertLife(c, units[3], state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, gc.IsNil)
	err = wp0.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "wordpress/4")
	c.Assert(err, gc.IsNil)
	assertLife(c, units[0], state.Dead)
	assertLife(c, units[4], state.Dying)
}

func (s *clientSuite) TestDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, gc.IsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.APIState.Client().DestroyServiceUnits("logging/0")
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be resposible for destroying the subordinate.)
	err = s.APIState.Client().DestroyServiceUnits("wordpress/0", "logging/0")
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, wordpress0, state.Dying)
	assertLife(c, logging0, state.Alive)
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
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
	} {
		c.Logf("test %s", url)
		err := s.APIState.Client().ServiceDeploy(
			url, "service", 1, "", constraints.Value{}, "",
		)
		c.Check(err, gc.ErrorMatches, expect)
		_, err = s.State.Service("service")
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

func (s *clientSuite) TestClientServiceDeployWithNetworks(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "dummy")
	cons := constraints.MustParse("mem=4G networks=^net3")

	// Check for invalid network tags handling.
	err := s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"net1", "net2"},
	)
	c.Assert(err, gc.ErrorMatches, `"net1" is not a valid network tag`)

	err = s.APIState.Client().ServiceDeployWithNetworks(
		curl.String(), "service", 3, "", cons, "",
		[]string{"network-net1", "network-net2"},
	)
	c.Assert(err, gc.IsNil)
	service := s.assertPrincipalDeployed(c, "service", curl, false, bundle, cons)

	networks, err := service.Networks()
	c.Assert(err, gc.IsNil)
	c.Assert(networks, gc.DeepEquals, []string{"net1", "net2"})
	serviceCons, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(serviceCons, gc.DeepEquals, cons)
}

func (s *clientSuite) assertPrincipalDeployed(c *gc.C, serviceName string, curl *charm.URL, forced bool, bundle charm.Charm, cons constraints.Value) *state.Service {
	service, err := s.State.Service(serviceName)
	c.Assert(err, gc.IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, gc.IsNil)
	c.Assert(force, gc.Equals, forced)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, bundle.Config())

	serviceCons, err := service.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(serviceCons, gc.DeepEquals, cons)
	units, err := service.AllUnits()
	c.Assert(err, gc.IsNil)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, gc.IsNil)
		machine, err := s.State.Machine(mid)
		c.Assert(err, gc.IsNil)
		machineCons, err := machine.Constraints()
		c.Assert(err, gc.IsNil)
		c.Assert(machineCons, gc.DeepEquals, cons)
	}
	return service
}

func (s *clientSuite) TestClientServiceDeployPrincipal(c *gc.C) {
	// TODO(fwereade): test ToMachineSpec directly on srvClient, when we
	// manage to extract it as a package and can thus do it conveniently.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "dummy")
	mem4g := constraints.MustParse("mem=4G")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g, "",
	)
	c.Assert(err, gc.IsNil)
	s.assertPrincipalDeployed(c, "service", curl, false, bundle, mem4g)
}

func (s *clientSuite) TestClientServiceDeploySubordinate(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "logging")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 0, "", constraints.Value{}, "",
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
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{}, "",
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
		curl.String(), "service-name", 1, "service-name:\n  skill-level: fred", constraints.Value{}, "",
	)
	c.Assert(err, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *clientSuite) TestClientServiceDeployToMachine(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "dummy")

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{}, machine.Id(),
	)
	c.Assert(err, gc.IsNil)

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
	c.Assert(units, gc.HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *clientSuite) TestClientServiceDeployServiceOwner(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")

	s.AddUser(c, "foobar")
	s.APIState = s.OpenAPIAs(c, "user-foobar", "password")

	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, gc.IsNil)

	service, err := s.State.Service("service")
	c.Assert(err, gc.IsNil)
	c.Assert(service.GetOwnerTag(), gc.Equals, "user-foobar")
}

func (s *clientSuite) deployServiceForTests(c *gc.C, store *coretesting.MockCharmStore) {
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(curl.String(),
		"service", 1, "", constraints.Value{}, "",
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
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for charmUrl, expect := range map[string]string{
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
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
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set minimum units for the service.
	minUnits := 2
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, minUnits)
}

func (s *clientSuite) TestClientServiceUpdateSetMinUnitsError(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set a negative minimum number of units for the service.
	minUnits := -1
	args := params.ServiceUpdate{
		ServiceName: "dummy",
		MinUnits:    &minUnits,
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for service "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(service.Refresh(), gc.IsNil)
	c.Assert(service.MinUnits(), gc.Equals, 0)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsStrings(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:     "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetSettingsYAML(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update settings for the service.
	args := params.ServiceUpdate{
		ServiceName:  "dummy",
		SettingsYAML: "dummy:\n  title: y-title\n  username: y-user",
	}
	err := s.APIState.Client().ServiceUpdate(args)
	c.Assert(err, gc.IsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := service.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.DeepEquals, expected)
}

func (s *clientSuite) TestClientServiceUpdateSetConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

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
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Calling ServiceUpdate with no parameters set is a no-op.
	args := params.ServiceUpdate{ServiceName: "wordpress"}
	err := s.APIState.Client().ServiceUpdate(args)
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
		curl.String(), "service", 3, "", constraints.Value{}, "",
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
		curl.String(), "service", 3, "", constraints.Value{}, "",
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
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for url, expect := range map[string]string{
		// TODO(fwereade,Makyo) make these errors consistent one day.
		"wordpress":                   "charm url series is not resolved",
		"cs:wordpress":                "charm url series is not resolved",
		"cs:precise/wordpress":        "charm url must include revision",
		"cs:precise/wordpress-999999": `cannot download charm ".*": charm not found in mock store: cs:precise/wordpress-999999`,
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
	return addSeriesCharm(c, store, "precise", name)
}

func addSeriesCharm(c *gc.C, store *coretesting.MockCharmStore, series, name string) (*charm.URL, charm.Charm) {
	bundle := coretesting.Charms.Bundle(c.MkDir(), name)
	scurl := fmt.Sprintf("cs:%s/%s-%d", series, name, bundle.Revision())
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
	c.Assert(relation.Refresh(), jc.Satisfies, errors.IsNotFound)
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
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	endpoints := []string{"riak", "wordpress"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
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
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))

	endpoints := []string{"riak:ring"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
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
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFound)

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
			Id:                      m.Id(),
			InstanceId:              "i-0",
			Status:                  params.StatusPending,
			Life:                    params.Alive,
			Series:                  "quantal",
			Jobs:                    []params.MachineJob{state.JobManageEnviron.ToParams()},
			Addresses:               []instance.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
		},
	}}) {
		c.Logf("got:")
		for _, d := range deltas {
			c.Logf("%#v\n", d.Entity)
		}
	}
}

func (s *clientSuite) TestClientSetServiceConstraints(c *gc.C) {
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

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
	service := s.AddTestingService(c, "dummy", s.AddTestingCharm(c, "dummy"))

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

func (s *clientSuite) TestClientServiceCharmRelations(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().ServiceCharmRelations("blah")
	c.Assert(err, gc.ErrorMatches, `service "blah" not found`)

	relations, err := s.APIState.Client().ServiceCharmRelations("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(relations, gc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *clientSuite) TestClientPublicAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PublicAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PublicAddress("0")
	c.Assert(err, gc.ErrorMatches, `machine "0" has no public address`)
	_, err = s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" has no public address`)
}

func (s *clientSuite) TestClientPublicAddressMachine(c *gc.C) {
	s.setUpScenario(c)

	// Internally, instance.SelectPublicAddress is used; the "most public"
	// address is returned.
	m1, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	cloudLocalAddress := instance.NewAddress("cloudlocal", instance.NetworkCloudLocal)
	publicAddress := instance.NewAddress("public", instance.NetworkPublic)
	err = m1.SetAddresses(cloudLocalAddress)
	c.Assert(err, gc.IsNil)
	addr, err := s.APIState.Client().PublicAddress("1")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
	err = m1.SetAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PublicAddress("1")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPublicAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	publicAddress := instance.NewAddress("public", instance.NetworkPublic)
	err = m1.SetAddresses(publicAddress)
	c.Assert(err, gc.IsNil)
	addr, err := s.APIState.Client().PublicAddress("wordpress/0")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "public")
}

func (s *clientSuite) TestClientPrivateAddressErrors(c *gc.C) {
	s.setUpScenario(c)
	_, err := s.APIState.Client().PrivateAddress("wordpress")
	c.Assert(err, gc.ErrorMatches, `unknown unit or machine "wordpress"`)
	_, err = s.APIState.Client().PrivateAddress("0")
	c.Assert(err, gc.ErrorMatches, `machine "0" has no internal address`)
	_, err = s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" has no internal address`)
}

func (s *clientSuite) TestClientPrivateAddress(c *gc.C) {
	s.setUpScenario(c)

	// Internally, instance.SelectInternalAddress is used; the public
	// address if no cloud-local one is available.
	m1, err := s.State.Machine("1")
	c.Assert(err, gc.IsNil)
	cloudLocalAddress := instance.NewAddress("cloudlocal", instance.NetworkCloudLocal)
	publicAddress := instance.NewAddress("public", instance.NetworkPublic)
	err = m1.SetAddresses(publicAddress)
	c.Assert(err, gc.IsNil)
	addr, err := s.APIState.Client().PrivateAddress("1")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "public")
	err = m1.SetAddresses(cloudLocalAddress, publicAddress)
	addr, err = s.APIState.Client().PrivateAddress("1")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "cloudlocal")
}

func (s *clientSuite) TestClientPrivateAddressUnit(c *gc.C) {
	s.setUpScenario(c)

	m1, err := s.State.Machine("1")
	privateAddress := instance.NewAddress("private", instance.NetworkCloudLocal)
	err = m1.SetAddresses(privateAddress)
	c.Assert(err, gc.IsNil)
	addr, err := s.APIState.Client().PrivateAddress("wordpress/0")
	c.Assert(err, gc.IsNil)
	c.Assert(addr, gc.Equals, "private")
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

func (s *clientSuite) TestClientSetEnvironAgentVersion(c *gc.C) {
	err := s.APIState.Client().SetEnvironAgentVersion(version.MustParse("9.8.7"))
	c.Assert(err, gc.IsNil)

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	agentVersion, found := envConfig.AllAttrs()["agent-version"]
	c.Assert(found, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, "9.8.7")
}

func (s *clientSuite) TestClientEnvironmentSetCannotChangeAgentVersion(c *gc.C) {
	args := map[string]interface{}{"agent-version": "9.9.9"}
	err := s.APIState.Client().EnvironmentSet(args)
	c.Assert(err, gc.ErrorMatches, "agent-version cannot be changed")
	// It's okay to pass env back with the same agent-version.
	cfg, err := s.APIState.Client().EnvironmentGet()
	c.Assert(err, gc.IsNil)
	c.Assert(cfg["agent-version"], gc.NotNil)
	err = s.APIState.Client().EnvironmentSet(cfg)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestClientEnvironmentUnset(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, gc.IsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	_, found := envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)

	err = s.APIState.Client().EnvironmentUnset("abc")
	c.Assert(err, gc.IsNil)
	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	_, found = envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsFalse)
}

func (s *clientSuite) TestClientEnvironmentUnsetMissing(c *gc.C) {
	// It's okay to unset a non-existent attribute.
	err := s.APIState.Client().EnvironmentUnset("not_there")
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestClientEnvironmentUnsetError(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"abc": 123}, nil, nil)
	c.Assert(err, gc.IsNil)
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	_, found := envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)

	// "type" may not be removed, and this will cause an error.
	// If any one attribute's removal causes an error, there
	// should be no change.
	err = s.APIState.Client().EnvironmentUnset("abc", "type")
	c.Assert(err, gc.ErrorMatches, "type: expected string, got nothing")
	envConfig, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	_, found = envConfig.AllAttrs()["abc"]
	c.Assert(found, jc.IsTrue)
}

func (s *clientSuite) TestClientFindTools(c *gc.C) {
	result, err := s.APIState.Client().FindTools(2, -1, "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, jc.Satisfies, params.IsCodeNotFound)
	toolstesting.UploadToStorage(c, s.Conn.Environ.Storage(), version.MustParseBinary("2.12.0-precise-amd64"))
	result, err = s.APIState.Client().FindTools(2, 12, "precise", "amd64")
	c.Assert(err, gc.IsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.List, gc.HasLen, 1)
	c.Assert(result.List[0].Version, gc.Equals, version.MustParseBinary("2.12.0-precise-amd64"))
}

func (s *clientSuite) checkMachine(c *gc.C, id, series, cons string) {
	// Ensure the machine was actually created.
	machine, err := s.BackingState.Machine(id)
	c.Assert(err, gc.IsNil)
	c.Assert(machine.Series(), gc.Equals, series)
	c.Assert(machine.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobHostUnits})
	machineConstraints, err := machine.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(machineConstraints.String(), gc.Equals, cons)
}

func (s *clientSuite) TestClientAddMachinesDefaultSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithSeries(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Series: "quantal",
			Jobs:   []params.MachineJob{params.JobHostUnits},
		}
	}
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, "quantal", apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachineInsideMachine(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{{
		Jobs:          []params.MachineJob{params.JobHostUnits},
		ContainerType: instance.LXC,
		ParentId:      "0",
		Series:        "quantal",
	}})
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxc/0")
}

func (s *clientSuite) TestClientAddMachinesWithConstraints(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		}
	}
	// The last machine has some constraints.
	apiParams[2].Constraints = constraints.MustParse("mem=4G")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
		s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
	}
}

func (s *clientSuite) TestClientAddMachinesWithPlacement(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 4)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		}
	}
	apiParams[0].Placement = instance.MustParsePlacement("lxc")
	apiParams[1].Placement = instance.MustParsePlacement("lxc:0")
	apiParams[1].ContainerType = instance.LXC
	apiParams[2].Placement = instance.MustParsePlacement("dummyenv:invalid")
	apiParams[3].Placement = instance.MustParsePlacement("dummyenv:valid")
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 4)
	c.Assert(machines[0].Machine, gc.Equals, "0/lxc/0")
	c.Assert(machines[1].Error, gc.ErrorMatches, "container type and placement are mutually exclusive")
	c.Assert(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: invalid placement is invalid")
	c.Assert(machines[3].Machine, gc.Equals, "1")

	m, err := s.BackingState.Machine(machines[3].Machine)
	c.Assert(err, gc.IsNil)
	c.Assert(m.Placement(), gc.DeepEquals, apiParams[3].Placement.Directive)
}

func (s *clientSuite) TestClientAddMachines1dot18(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		}
	}
	apiParams[1].ContainerType = instance.LXC
	apiParams[1].ParentId = "0"
	machines, err := s.APIState.Client().AddMachines1dot18(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 2)
	c.Assert(machines[0].Machine, gc.Equals, "0")
	c.Assert(machines[1].Machine, gc.Equals, "0/lxc/0")
}

func (s *clientSuite) TestClientAddMachines1dot18SomeErrors(c *gc.C) {
	apiParams := []params.AddMachineParams{{
		Jobs:     []params.MachineJob{params.JobHostUnits},
		ParentId: "123",
	}}
	machines, err := s.APIState.Client().AddMachines1dot18(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 1)
	c.Check(machines[0].Error, gc.ErrorMatches, "parent machine specified without container type")
}

func (s *clientSuite) TestClientAddMachinesSomeErrors(c *gc.C) {
	// Here we check that adding a number of containers correctly handles the
	// case that some adds succeed and others fail and report the errors
	// accordingly.
	// We will set up params to the AddMachines API to attempt to create 3 machines.
	// Machines 0 and 1 will be added successfully.
	// Remaining machines will fail due to different reasons.

	// Create a machine to host the requested containers.
	host, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	// The host only supports lxc containers.
	err = host.SetSupportedContainers([]instance.ContainerType{instance.LXC})
	c.Assert(err, gc.IsNil)

	// Set up params for adding 3 containers.
	apiParams := make([]params.AddMachineParams, 3)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Jobs: []params.MachineJob{params.JobHostUnits},
		}
	}
	// This will cause a machine add to fail due to an unsupported container.
	apiParams[2].ContainerType = instance.KVM
	apiParams[2].ParentId = host.Id()
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 3)

	// Check the results - machines 2 and 3 will have errors.
	c.Check(machines[0].Machine, gc.Equals, "1")
	c.Check(machines[0].Error, gc.IsNil)
	c.Check(machines[1].Machine, gc.Equals, "2")
	c.Check(machines[1].Error, gc.IsNil)
	c.Check(machines[2].Error, gc.ErrorMatches, "cannot add a new machine: machine 0 cannot host kvm containers")
}

func (s *clientSuite) TestClientAddMachinesWithInstanceIdSomeErrors(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 3)
	addrs := []instance.Address{instance.NewAddress("1.2.3.4", instance.NetworkUnknown)}
	hc := instance.MustParseHardware("mem=4G")
	for i := 0; i < 3; i++ {
		apiParams[i] = params.AddMachineParams{
			Jobs:       []params.MachineJob{params.JobHostUnits},
			InstanceId: instance.Id(fmt.Sprintf("1234-%d", i)),
			Nonce:      "foo",
			HardwareCharacteristics: hc,
			Addrs: addrs,
		}
	}
	// This will cause the last machine add to fail.
	apiParams[2].Nonce = ""
	machines, err := s.APIState.Client().AddMachines(apiParams)
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 3)
	for i, machineResult := range machines {
		if i == 2 {
			c.Assert(machineResult.Error, gc.NotNil)
			c.Assert(machineResult.Error, gc.ErrorMatches, "cannot add a new machine: cannot add a machine with an instance id and no nonce")
		} else {
			c.Assert(machineResult.Machine, gc.DeepEquals, strconv.Itoa(i))
			s.checkMachine(c, machineResult.Machine, coretesting.FakeDefaultSeries, apiParams[i].Constraints.String())
			instanceId := fmt.Sprintf("1234-%d", i)
			s.checkInstance(c, machineResult.Machine, instanceId, "foo", hc, addrs)
		}
	}
}

func (s *clientSuite) checkInstance(c *gc.C, id, instanceId, nonce string,
	hc instance.HardwareCharacteristics, addr []instance.Address) {

	machine, err := s.BackingState.Machine(id)
	c.Assert(err, gc.IsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(machine.CheckProvisioned(nonce), jc.IsTrue)
	c.Assert(machineInstanceId, gc.Equals, instance.Id(instanceId))
	machineHardware, err := machine.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(machineHardware.String(), gc.Equals, hc.String())
	c.Assert(machine.Addresses(), gc.DeepEquals, addr)
}

func (s *clientSuite) TestInjectMachinesStillExists(c *gc.C) {
	results := new(params.AddMachinesResults)
	// We need to use Call directly because the client interface
	// no longer refers to InjectMachine.
	args := params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Jobs:       []params.MachineJob{params.JobHostUnits},
			InstanceId: "i-foo",
			Nonce:      "nonce",
		}},
	}
	err := s.APIState.Call("Client", "", "AddMachines", args, &results)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Machines, gc.HasLen, 1)
}

func (s *clientSuite) TestProvisioningScript(c *gc.C) {
	// Inject a machine and then call the ProvisioningScript API.
	// The result should be the same as when calling MachineConfig,
	// converting it to a cloudinit.MachineConfig, and disabling
	// apt_upgrade.
	apiParams := params.AddMachineParams{
		Jobs:       []params.MachineJob{params.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine
	// Call ProvisioningScript. Normally ProvisioningScript and
	// MachineConfig are mutually exclusive; both of them will
	// allocate a state/api password for the machine agent.
	script, err := s.APIState.Client().ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     apiParams.Nonce,
	})
	c.Assert(err, gc.IsNil)
	mcfg, err := client.MachineConfig(s.State, machineId, apiParams.Nonce, "")
	c.Assert(err, gc.IsNil)
	sshinitScript, err := manual.ProvisioningScript(mcfg)
	c.Assert(err, gc.IsNil)
	// ProvisioningScript internally calls MachineConfig,
	// which allocates a new, random password. Everything
	// about the scripts should be the same other than
	// the line containing "oldpassword" from agent.conf.
	scriptLines := strings.Split(script, "\n")
	sshinitScriptLines := strings.Split(sshinitScript, "\n")
	c.Assert(scriptLines, gc.HasLen, len(sshinitScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, gc.Equals, sshinitScriptLines[i])
	}
}

func (s *clientSuite) TestProvisioningScriptDisablePackageCommands(c *gc.C) {
	apiParams := params.AddMachineParams{
		Jobs:       []params.MachineJob{params.JobHostUnits},
		InstanceId: instance.Id("1234"),
		Nonce:      "foo",
		HardwareCharacteristics: instance.MustParseHardware("arch=amd64"),
	}
	machines, err := s.APIState.Client().AddMachines([]params.AddMachineParams{apiParams})
	c.Assert(err, gc.IsNil)
	c.Assert(len(machines), gc.Equals, 1)
	machineId := machines[0].Machine
	for _, disable := range []bool{false, true} {
		script, err := s.APIState.Client().ProvisioningScript(params.ProvisioningScriptParams{
			MachineId: machineId,
			Nonce:     apiParams.Nonce,
			DisablePackageCommands: disable,
		})
		c.Assert(err, gc.IsNil)
		var checker gc.Checker = jc.Contains
		if disable {
			// We disabled package commands: there should be no "apt" commands in the script.
			checker = gc.Not(checker)
		}
		c.Assert(script, checker, "apt-get")
	}
}

func (s *clientSuite) TestClientSpecializeStoreOnDeployServiceSetCharmAndAddCharm(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()

	attrs := map[string]interface{}{"charm-store-auth": "token=value",
		"test-mode": true}
	err := s.State.UpdateEnvironConfig(attrs, nil, nil)
	c.Assert(err, gc.IsNil)

	curl, _ := addCharm(c, store, "dummy")
	err = s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{}, "",
	)
	c.Assert(err, gc.IsNil)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs, gc.Equals, "token=value")
	c.Assert(store.TestMode, gc.Equals, true)

	store.AuthAttrs = ""

	curl, _ = addCharm(c, store, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", curl.String(), false,
	)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs, gc.Equals, "token=value")

	curl, _ = addCharm(c, store, "riak")
	err = s.APIState.Client().AddCharm(curl)

	// check that the store's auth attributes were set
	c.Assert(store.AuthAttrs, gc.Equals, "token=value")
}

func (s *clientSuite) TestAddCharm(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()

	client := s.APIState.Client()
	// First test the sanity checks.
	err := client.AddCharm(&charm.URL{Reference: charm.Reference{Name: "nonsense"}})
	c.Assert(err, gc.ErrorMatches, `charm URL has invalid schema: ":nonsense-0"`)
	err = client.AddCharm(charm.MustParseURL("local:precise/dummy"))
	c.Assert(err, gc.ErrorMatches, "only charm store charm URLs are supported, with cs: schema")
	err = client.AddCharm(charm.MustParseURL("cs:precise/wordpress"))
	c.Assert(err, gc.ErrorMatches, "charm URL must include revision")

	// Add a charm, without uploading it to storage, to
	// check that AddCharm does not try to do it.
	charmDir := coretesting.Charms.Dir("dummy")
	ident := fmt.Sprintf("%s-%d", charmDir.Meta().Name, charmDir.Revision())
	curl := charm.MustParseURL("cs:quantal/" + ident)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/" + ident)
	c.Assert(err, gc.IsNil)
	sch, err := s.State.AddCharm(charmDir, curl, bundleURL, ident+"-sha256")
	c.Assert(err, gc.IsNil)

	name := charm.Quote(sch.URL().String())
	storage := s.Conn.Environ.Storage()
	_, err = storage.Get(name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// AddCharm should see the charm in state and not upload it.
	err = client.AddCharm(sch.URL())
	c.Assert(err, gc.IsNil)
	_, err = storage.Get(name)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Now try adding another charm completely.
	curl, _ = addCharm(c, store, "wordpress")
	err = client.AddCharm(curl)
	c.Assert(err, gc.IsNil)

	// Verify it's in state and it got uploaded.
	sch, err = s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
	s.assertUploaded(c, storage, sch.BundleURL(), sch.BundleSha256())
}

var resolveCharmCases = []struct {
	schema, defaultSeries, charmName string
	parseErr                         string
	resolveErr                       string
}{
	{"cs", "precise", "wordpress", "", ""},
	{"cs", "trusty", "wordpress", "", ""},
	{"cs", "", "wordpress", "", `missing default series, cannot resolve charm url: "cs:wordpress"`},
	{"cs", "trusty", "", `charm URL has invalid charm name: "cs:"`, ""},
	{"local", "trusty", "wordpress", "", `only charm store charm references are supported, with cs: schema`},
	{"cs", "precise", "hl3", "", ""},
	{"cs", "trusty", "hl3", "", ""},
	{"cs", "", "hl3", "", `missing default series, cannot resolve charm url: \"cs:hl3\"`},
}

func (s *clientSuite) TestResolveCharm(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()

	for i, test := range resolveCharmCases {
		c.Logf("test %d: %#v", i, test)
		// Mock charm store will use this to resolve a charm reference.
		store.DefaultSeries = test.defaultSeries

		client := s.APIState.Client()
		ref, series, err := charm.ParseReference(fmt.Sprintf("%s:%s", test.schema, test.charmName))
		if test.parseErr == "" {
			if !c.Check(err, gc.IsNil) {
				continue
			}
		} else {
			c.Assert(err, gc.NotNil)
			c.Check(err, gc.ErrorMatches, test.parseErr)
			continue
		}
		c.Check(series, gc.Equals, "")
		c.Check(ref.String(), gc.Equals, fmt.Sprintf("%s:%s", test.schema, test.charmName))

		curl, err := client.ResolveCharm(ref)
		if err == nil {
			c.Assert(curl, gc.NotNil)
			// Only cs: schema should make it through here
			c.Check(curl.String(), gc.Equals, fmt.Sprintf("cs:%s/%s", test.defaultSeries, test.charmName))
			c.Check(test.resolveErr, gc.Equals, "")
		} else {
			c.Check(curl, gc.IsNil)
			c.Check(err, gc.ErrorMatches, test.resolveErr)
		}
	}
}

func (s *clientSuite) TestAddCharmConcurrently(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()

	client := s.APIState.Client()
	curl, _ := addCharm(c, store, "wordpress")

	// Expect storage Put() to be called once for each goroutine
	// below.
	ops := make(chan dummy.Operation, 500)
	dummy.Listen(ops)
	go s.assertPutCalled(c, ops, 10)

	// Try adding the same charm concurrently from multiple goroutines
	// to test no "duplicate key errors" are reported (see lp bug
	// #1067979) and also at the end only one charm document is
	// created.

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			c.Assert(client.AddCharm(curl), gc.IsNil, gc.Commentf("goroutine %d", index))
			sch, err := s.State.Charm(curl)
			c.Assert(err, gc.IsNil, gc.Commentf("goroutine %d", index))
			c.Assert(sch.URL(), jc.DeepEquals, curl, gc.Commentf("goroutine %d", index))
			expectedName := fmt.Sprintf("%s-%d-[0-9a-f-]+", curl.Name, curl.Revision)
			c.Assert(getArchiveName(sch.BundleURL()), gc.Matches, expectedName)
		}(i)
	}
	wg.Wait()
	close(ops)

	// Verify there is only a single uploaded charm remains and it
	// contains the correct data.
	sch, err := s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
	storage, err := environs.GetStorage(s.State)
	c.Assert(err, gc.IsNil)
	uploads, err := storage.List(fmt.Sprintf("%s-%d-", curl.Name, curl.Revision))
	c.Assert(err, gc.IsNil)
	c.Assert(uploads, gc.HasLen, 1)
	c.Assert(getArchiveName(sch.BundleURL()), gc.Equals, uploads[0])
	s.assertUploaded(c, storage, sch.BundleURL(), sch.BundleSha256())
}

func (s *clientSuite) TestAddCharmOverwritesPlaceholders(c *gc.C) {
	store, restore := makeMockCharmStore()
	defer restore()

	client := s.APIState.Client()
	curl, _ := addCharm(c, store, "wordpress")

	// Add a placeholder with the same charm URL.
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, gc.IsNil)
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Now try to add the charm, which will convert the placeholder to
	// a pending charm.
	err = client.AddCharm(curl)
	c.Assert(err, gc.IsNil)

	// Make sure the document's flags were reset as expected.
	sch, err := s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), jc.DeepEquals, curl)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *clientSuite) TestCharmArchiveName(c *gc.C) {
	for rev, name := range []string{"Foo", "bar", "wordpress", "mysql"} {
		archiveFormat := fmt.Sprintf("%s-%d-[0-9a-f-]+", name, rev)
		archiveName, err := client.CharmArchiveName(name, rev)
		c.Check(err, gc.IsNil)
		c.Check(archiveName, gc.Matches, archiveFormat)
	}
}

func (s *clientSuite) assertPutCalled(c *gc.C, ops chan dummy.Operation, numCalls int) {
	calls := 0
	select {
	case op, ok := <-ops:
		if !ok {
			return
		}
		if op, ok := op.(dummy.OpPutFile); ok {
			calls++
			if calls > numCalls {
				c.Fatalf("storage Put() called %d times, expected %d times", calls, numCalls)
				return
			}
			nameFormat := "[0-9a-z-]+-[0-9]+-[0-9a-f-]+"
			c.Assert(op.FileName, gc.Matches, nameFormat)
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for a storage Put() calls")
		return
	}
}

func (s *clientSuite) assertUploaded(c *gc.C, storage envstorage.Storage, bundleURL *url.URL, expectedSHA256 string) {
	archiveName := getArchiveName(bundleURL)
	reader, err := storage.Get(archiveName)
	c.Assert(err, gc.IsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, gc.IsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func getArchiveName(bundleURL *url.URL) string {
	return strings.TrimPrefix(bundleURL.RequestURI(), "/dummyenv/private/")
}

func (s *clientSuite) TestRetryProvisioning(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetStatus(params.StatusError, "error", nil)
	c.Assert(err, gc.IsNil)
	_, err = s.APIState.Client().RetryProvisioning(machine.Tag())
	c.Assert(err, gc.IsNil)

	status, info, data, err := machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusError)
	c.Assert(info, gc.Equals, "error")
	c.Assert(data["transient"], gc.Equals, true)
}

func (s *clientSuite) setAgentAlive(c *gc.C, machineId string) *presence.Pinger {
	m, err := s.BackingState.Machine(machineId)
	c.Assert(err, gc.IsNil)
	pinger, err := m.SetAgentAlive()
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
	err = m.WaitAgentAlive(coretesting.LongWait)
	c.Assert(err, gc.IsNil)
	return pinger
}

var (
	emptyCons     = constraints.Value{}
	defaultSeries = ""
)

func (s *clientSuite) TestClientEnsureAvailabilitySeries(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	// We have to ensure the agents are alive, or EnsureAvailability will
	// create more to replace them.
	pinger := s.setAgentAlive(c, "0")
	defer pinger.Kill()
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	err = s.APIState.Client().EnsureAvailability(3, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err = s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
	defer s.setAgentAlive(c, "1").Kill()
	defer s.setAgentAlive(c, "2").Kill()
	err = s.APIState.Client().EnsureAvailability(5, emptyCons, "non-default")
	c.Assert(err, gc.IsNil)
	machines, err = s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 5)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
	c.Assert(machines[3].Series(), gc.Equals, "non-default")
	c.Assert(machines[4].Series(), gc.Equals, "non-default")
}

func (s *clientSuite) TestClientEnsureAvailabilityConstraints(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	pinger := s.setAgentAlive(c, "0")
	defer pinger.Kill()
	err = s.APIState.Client().EnsureAvailability(
		3, constraints.MustParse("mem=4G"), defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		constraints.Value{},
		constraints.MustParse("mem=4G"),
		constraints.MustParse("mem=4G"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, gc.IsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestClientEnsureAvailability0Preserves(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	pinger := s.setAgentAlive(c, "0")
	defer pinger.Kill()
	// A value of 0 says either "if I'm not HA, make me HA" or "preserve my
	// current HA settings".
	err = s.APIState.Client().EnsureAvailability(0, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err := s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 3)
	defer s.setAgentAlive(c, "1").Kill()
	// Now, we keep agent 1 alive, but not agent 2, calling
	// EnsureAvailability(0) again will cause us to start another machine
	err = s.APIState.Client().EnsureAvailability(0, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err = s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 4)
}

func (s *clientSuite) TestClientEnsureAvailability0Preserves5(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	pinger := s.setAgentAlive(c, "0")
	defer pinger.Kill()
	// Start off with 5 servers
	err = s.APIState.Client().EnsureAvailability(5, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err := s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 5)
	defer s.setAgentAlive(c, "1").Kill()
	defer s.setAgentAlive(c, "2").Kill()
	defer s.setAgentAlive(c, "3").Kill()
	// Keeping all alive but one, will bring up 1 more server to preserve 5
	err = s.APIState.Client().EnsureAvailability(0, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	machines, err = s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 6)
}

func (s *clientSuite) TestClientEnsureAvailabilityErrors(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	pinger := s.setAgentAlive(c, "0")
	defer pinger.Kill()
	err = s.APIState.Client().EnsureAvailability(-1, emptyCons, defaultSeries)
	c.Assert(err, gc.ErrorMatches, "number of state servers must be odd and non-negative")
	err = s.APIState.Client().EnsureAvailability(3, emptyCons, defaultSeries)
	c.Assert(err, gc.IsNil)
	err = s.APIState.Client().EnsureAvailability(1, emptyCons, defaultSeries)
	c.Assert(err, gc.ErrorMatches, "cannot reduce state server count")
}

func (s *clientSuite) TestAPIHostPorts(c *gc.C) {
	apiHostPorts, err := s.APIState.Client().APIHostPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(apiHostPorts, gc.HasLen, 0)

	server1Addresses := []instance.Address{{
		Value:        "server-1",
		Type:         instance.HostName,
		NetworkScope: instance.NetworkPublic,
	}, {
		Value:        "10.0.0.1",
		Type:         instance.Ipv4Address,
		NetworkName:  "internal",
		NetworkScope: instance.NetworkCloudLocal,
	}}
	server2Addresses := []instance.Address{{
		Value:        "::1",
		Type:         instance.Ipv6Address,
		NetworkName:  "loopback",
		NetworkScope: instance.NetworkMachineLocal,
	}}
	stateAPIHostPorts := [][]instance.HostPort{
		instance.AddressesWithPort(server1Addresses, 123),
		instance.AddressesWithPort(server2Addresses, 456),
	}

	err = s.State.SetAPIHostPorts(stateAPIHostPorts)
	c.Assert(err, gc.IsNil)
	apiHostPorts, err = s.APIState.Client().APIHostPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(apiHostPorts, gc.DeepEquals, stateAPIHostPorts)
}

func (s *clientSuite) TestClientAgentVersion(c *gc.C) {
	current := version.MustParse("1.2.0")
	s.PatchValue(&version.Current.Number, current)
	result, err := s.APIState.Client().AgentVersion()
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, current)
}
