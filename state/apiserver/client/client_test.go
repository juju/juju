// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	. "launchpad.net/gocheck"
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

var _ = Suite(&clientSuite{})

func (s *clientSuite) TestClientStatus(c *C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status()
	c.Assert(err, IsNil)
	c.Assert(status, DeepEquals, scenarioStatus)
}

func (s *clientSuite) TestClientServerSet(c *C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "xxx",
		"username": "yyy",
	})
	c.Assert(err, IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{
		"title":    "xxx",
		"username": "yyy",
	})
}

func (s *clientSuite) TestClientServiceSetYAML(c *C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	err = s.APIState.Client().ServiceSetYAML("dummy", "dummy:\n  title: aaa\n  username: bbb")
	c.Assert(err, IsNil)
	settings, err := dummy.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{
		"title":    "aaa",
		"username": "bbb",
	})
}

var clientAddServiceUnitsTests = []struct {
	about    string
	expected []string
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
}

func (s *clientSuite) TestClientAddServiceUnits(c *C) {
	_, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	for i, t := range clientAddServiceUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		units, err := s.APIState.Client().AddServiceUnits("dummy", len(t.expected))
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
			continue
		}
		c.Assert(err, IsNil)
		c.Assert(units, DeepEquals, t.expected)
	}
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

func (s *clientSuite) TestClientCharmInfo(c *C) {
	// Use wordpress for tests so that we can compare Provides and Requires.
	charm := s.AddTestingCharm(c, "wordpress")
	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)
		info, err := s.APIState.Client().CharmInfo(t.url)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
			continue
		}
		c.Assert(err, IsNil)
		expected := &api.CharmInfo{
			Revision: charm.Revision(),
			URL:      charm.URL().String(),
			Config:   charm.Config(),
			Meta:     charm.Meta(),
		}
		c.Assert(info, DeepEquals, expected)
	}
}

func (s *clientSuite) TestClientEnvironmentInfo(c *C) {
	conf, _ := s.State.EnvironConfig()
	info, err := s.APIState.Client().EnvironmentInfo()
	c.Assert(err, IsNil)
	env, err := s.State.Environment()
	c.Assert(err, IsNil)
	c.Assert(info.DefaultSeries, Equals, conf.DefaultSeries())
	c.Assert(info.ProviderType, Equals, conf.Type())
	c.Assert(info.Name, Equals, conf.Name())
	c.Assert(info.UUID, Equals, env.UUID())
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

func (s *clientSuite) TestClientAnnotations(c *C) {
	// Set up entities.
	service, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)
	machine, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	environment, err := s.State.Environment()
	c.Assert(err, IsNil)
	entities := []state.TaggedAnnotator{service, unit, machine, environment}
	for i, t := range clientAnnotationsTests {
		for _, entity := range entities {
			id := entity.Tag()
			c.Logf("test %d. %s. entity %s", i, t.about, id)
			// Set initial entity annotations.
			err := entity.SetAnnotations(t.initial)
			c.Assert(err, IsNil)
			// Add annotations using the API call.
			err = s.APIState.Client().SetAnnotations(id, t.input)
			if t.err != "" {
				c.Assert(err, ErrorMatches, t.err)
				continue
			}
			// Check annotations are correctly set.
			dbann, err := entity.Annotations()
			c.Assert(err, IsNil)
			c.Assert(dbann, DeepEquals, t.expected)
			// Retrieve annotations using the API call.
			ann, err := s.APIState.Client().GetAnnotations(id)
			c.Assert(err, IsNil)
			// Check annotations are correctly returned.
			c.Assert(ann, DeepEquals, dbann)
			// Clean up annotations on the current entity.
			cleanup := make(map[string]string)
			for key := range dbann {
				cleanup[key] = ""
			}
			err = entity.SetAnnotations(cleanup)
			c.Assert(err, IsNil)
		}
	}
}

func (s *clientSuite) TestClientAnnotationsBadEntity(c *C) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo", "service-", "service-foo/bar"}
	expected := `invalid entity tag ".*"`
	for _, id := range bad {
		err := s.APIState.Client().SetAnnotations(id, map[string]string{"mykey": "myvalue"})
		c.Assert(err, ErrorMatches, expected)
		_, err = s.APIState.Client().GetAnnotations(id)
		c.Assert(err, ErrorMatches, expected)
	}
}

func (s *clientSuite) TestClientServiceGet(c *C) {
	s.setUpScenario(c)
	results, err := s.APIState.Client().ServiceGet("wordpress")
	c.Assert(err, IsNil)
	c.Assert(results, DeepEquals, &params.ServiceGetResults{
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

func (s *clientSuite) TestClientServiceExpose(c *C) {
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, IsNil)
	c.Assert(service.IsExposed(), Equals, false)
	err = s.APIState.Client().ServiceExpose(serviceName)
	c.Assert(err, IsNil)
	err = service.Refresh()
	c.Assert(err, IsNil)
	c.Assert(service.IsExposed(), Equals, true)
}

func (s *clientSuite) TestClientServiceUnexpose(c *C) {
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, IsNil)
	service.SetExposed()
	c.Assert(service.IsExposed(), Equals, true)
	err = s.APIState.Client().ServiceUnexpose(serviceName)
	c.Assert(err, IsNil)
	service.Refresh()
	c.Assert(service.IsExposed(), Equals, false)
}

func (s *clientSuite) TestClientServiceDestroy(c *C) {
	// Setup:
	s.setUpScenario(c)
	serviceName := "wordpress"
	service, err := s.State.Service(serviceName)
	c.Assert(err, IsNil)
	// Code under test:
	err = s.APIState.Client().ServiceDestroy(serviceName)
	c.Assert(err, IsNil)
	err = service.Refresh()
	// The test actual assertion: the service should no-longer be Alive.
	c.Assert(service.Life(), Not(Equals), state.Alive)
}

func (s *clientSuite) TestClientUnitResolved(c *C) {
	// Setup:
	s.setUpScenario(c)
	u, err := s.State.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = u.SetStatus(params.StatusError, "gaaah")
	c.Assert(err, IsNil)
	// Code under test:
	err = s.APIState.Client().Resolved("wordpress/0", false)
	c.Assert(err, IsNil)
	// Freshen the unit's state.
	err = u.Refresh()
	c.Assert(err, IsNil)
	// And now the actual test assertions: we set the unit as resolved via
	// the API so it should have a resolved mode set.
	mode := u.Resolved()
	c.Assert(mode, Equals, state.ResolvedNoHooks)
}

func (s *clientSuite) TestClientServiceDeployCharmErrors(c *C) {
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
		c.Check(err, ErrorMatches, expect)
		_, err = s.State.Service("service")
		c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
	}
}

func (s *clientSuite) TestClientServiceDeployPrincipal(c *C) {
	// TODO(fwereade): test ToMachineSpec directly on srvClient, when we
	// manage to extract it as a package and can thus do it conveniently.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "dummy")
	mem4g := constraints.MustParse("mem=4G")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", mem4g,
	)
	c.Assert(err, IsNil)
	service, err := s.State.Service("service")
	c.Assert(err, IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, false)
	c.Assert(charm.URL(), DeepEquals, curl)
	c.Assert(charm.Meta(), DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), DeepEquals, bundle.Config())

	cons, err := service.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, mem4g)
	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	for _, unit := range units {
		mid, err := unit.AssignedMachineId()
		c.Assert(err, IsNil)
		machine, err := s.State.Machine(mid)
		c.Assert(err, IsNil)
		cons, err := machine.Constraints()
		c.Assert(err, IsNil)
		c.Assert(cons, DeepEquals, mem4g)
	}
}

func (s *clientSuite) TestClientServiceDeploySubordinate(c *C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, bundle := addCharm(c, store, "logging")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 0, "", constraints.Value{},
	)
	service, err := s.State.Service("service-name")
	c.Assert(err, IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, IsNil)
	c.Assert(force, Equals, false)
	c.Assert(charm.URL(), DeepEquals, curl)
	c.Assert(charm.Meta(), DeepEquals, bundle.Meta())
	c.Assert(charm.Config(), DeepEquals, bundle.Config())

	units, err := service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 0)
}

func (s *clientSuite) TestClientServiceDeployConfig(c *C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  username: fred", constraints.Value{},
	)
	c.Assert(err, IsNil)
	service, err := s.State.Service("service-name")
	c.Assert(err, IsNil)
	settings, err := service.ConfigSettings()
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, charm.Settings{"username": "fred"})
}

func (s *clientSuite) TestClientServiceDeployConfigError(c *C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service-name", 1, "service-name:\n  skill-level: fred", constraints.Value{},
	)
	c.Assert(err, ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Service("service-name")
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

func (s *clientSuite) TestClientServiceSetCharm(c *C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{},
	)
	c.Assert(err, IsNil)
	addCharm(c, store, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", false,
	)
	c.Assert(err, IsNil)

	// Ensure that the charm is not marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, IsNil)
	c.Assert(charm.URL().String(), Equals, "cs:precise/wordpress-3")
	c.Assert(force, Equals, false)
}

func (s *clientSuite) TestClientServiceSetCharmForce(c *C) {
	store, restore := makeMockCharmStore()
	defer restore()
	curl, _ := addCharm(c, store, "dummy")
	err := s.APIState.Client().ServiceDeploy(
		curl.String(), "service", 3, "", constraints.Value{},
	)
	c.Assert(err, IsNil)
	addCharm(c, store, "wordpress")
	err = s.APIState.Client().ServiceSetCharm(
		"service", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, IsNil)

	// Ensure that the charm is marked as forced.
	service, err := s.State.Service("service")
	c.Assert(err, IsNil)
	charm, force, err := service.Charm()
	c.Assert(err, IsNil)
	c.Assert(charm.URL().String(), Equals, "cs:precise/wordpress-3")
	c.Assert(force, Equals, true)
}

func (s *clientSuite) TestClientServiceSetCharmInvalidService(c *C) {
	_, restore := makeMockCharmStore()
	defer restore()
	err := s.APIState.Client().ServiceSetCharm(
		"badservice", "cs:precise/wordpress-3", true,
	)
	c.Assert(err, ErrorMatches, `service "badservice" not found`)
}

func (s *clientSuite) TestClientServiceSetCharmErrors(c *C) {
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
		c.Check(err, ErrorMatches, expect)
	}
}

func makeMockCharmStore() (store *coretesting.MockCharmStore, restore func()) {
	mockStore := coretesting.NewMockCharmStore()
	origStore := client.CharmStore
	client.CharmStore = mockStore
	return mockStore, func() { client.CharmStore = origStore }
}

func addCharm(c *C, store *coretesting.MockCharmStore, name string) (*charm.URL, charm.Charm) {
	bundle := coretesting.Charms.Bundle(c.MkDir(), name)
	scurl := fmt.Sprintf("cs:precise/%s-%d", name, bundle.Revision())
	curl := charm.MustParseURL(scurl)
	err := store.SetCharm(curl, bundle)
	c.Assert(err, IsNil)
	return curl, bundle
}

func (s *clientSuite) TestSuccessfulAddRelation(c *C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "mysql"}
	res, err := s.APIState.Client().AddRelation(endpoints...)
	c.Assert(err, IsNil)
	c.Assert(res.Endpoints["wordpress"].Name, Equals, "db")
	c.Assert(res.Endpoints["wordpress"].Interface, Equals, "mysql")
	c.Assert(res.Endpoints["wordpress"].Scope, Equals, charm.RelationScope("global"))
	c.Assert(res.Endpoints["mysql"].Name, Equals, "server")
	c.Assert(res.Endpoints["mysql"].Interface, Equals, "mysql")
	c.Assert(res.Endpoints["mysql"].Scope, Equals, charm.RelationScope("global"))
	for _, endpoint := range endpoints {
		svc, err := s.State.Service(endpoint)
		c.Assert(err, IsNil)
		rels, err := svc.Relations()
		c.Assert(err, IsNil)
		for _, rel := range rels {
			c.Assert(rel.Life(), Equals, state.Alive)
		}
	}
}

func (s *clientSuite) TestSuccessfulDestroyRelation(c *C) {
	s.setUpScenario(c)
	endpoints := []string{"wordpress", "logging"}
	err := s.APIState.Client().DestroyRelation(endpoints...)
	c.Assert(err, IsNil)
	for _, endpoint := range endpoints {
		service, err := s.State.Service(endpoint)
		c.Assert(err, IsNil)
		rels, err := service.Relations()
		c.Assert(err, IsNil)
		// When relations are destroyed they don't go away immediately but
		// instead are set to 'Dying', due to references held by the user
		// agent.
		for _, rel := range rels {
			c.Assert(rel.Life(), Equals, state.Dying)
		}
	}
}

func (s *clientSuite) TestNoRelation(c *C) {
	s.setUpScenario(c)
	err := s.APIState.Client().DestroyRelation("wordpress", "mysql")
	c.Assert(err, ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *clientSuite) TestClientWatchAll(c *C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	err = m.SetProvisioned("i-0", state.BootstrapNonce, nil)
	c.Assert(err, IsNil)
	watcher, err := s.APIState.Client().WatchAll()
	c.Assert(err, IsNil)
	defer func() {
		err := watcher.Stop()
		c.Assert(err, IsNil)
	}()
	deltas, err := watcher.Next()
	c.Assert(err, IsNil)
	if !c.Check(deltas, DeepEquals, []params.Delta{{
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
