// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
)

func (s *suite) TestClientStatus(c *C) {
	s.setUpScenario(c)
	status, err := s.APIState.Client().Status()
	c.Assert(err, IsNil)
	c.Assert(status, DeepEquals, scenarioStatus)
}

func (s *suite) TestClientServerSet(c *C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	err = s.APIState.Client().ServiceSet("dummy", map[string]string{
		"title":    "xxx",
		"username": "yyy",
	})
	c.Assert(err, IsNil)
	conf, err := dummy.Config()
	c.Assert(err, IsNil)
	c.Assert(conf.Map(), DeepEquals, map[string]interface{}{
		"title":    "xxx",
		"username": "yyy",
	})
}

func (s *suite) TestClientServiceSetYAML(c *C) {
	dummy, err := s.State.AddService("dummy", s.AddTestingCharm(c, "dummy"))
	c.Assert(err, IsNil)
	err = s.APIState.Client().ServiceSetYAML("dummy", "title: aaa\nusername: bbb")
	c.Assert(err, IsNil)
	conf, err := dummy.Config()
	c.Assert(err, IsNil)
	c.Assert(conf.Map(), DeepEquals, map[string]interface{}{
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

func (s *suite) TestClientAddServiceUnits(c *C) {
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

func (s *suite) TestClientCharmInfo(c *C) {
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

func (s *suite) TestClientEnvironmentInfo(c *C) {
	conf, _ := s.State.EnvironConfig()
	info, err := s.APIState.Client().EnvironmentInfo()
	c.Assert(err, IsNil)
	c.Assert(info.DefaultSeries, Equals, conf.DefaultSeries())
	c.Assert(info.ProviderType, Equals, conf.Type())
	c.Assert(info.Name, Equals, conf.Name())
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

func (s *suite) TestClientAnnotations(c *C) {
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

func (s *suite) TestClientAnnotationsBadEntity(c *C) {
	bad := []string{"", "machine", "-foo", "foo-", "---", "machine-jim", "unit-123", "unit-foo", "service-", "service-foo/bar"}
	expected := `invalid entity tag ".*"`
	for _, id := range bad {
		err := s.APIState.Client().SetAnnotations(id, map[string]string{"mykey": "myvalue"})
		c.Assert(err, ErrorMatches, expected)
		_, err = s.APIState.Client().GetAnnotations(id)
		c.Assert(err, ErrorMatches, expected)
	}
}

func (s *suite) TestClientServiceGet(c *C) {
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

func (s *suite) TestClientServiceExpose(c *C) {
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

func (s *suite) TestClientServiceUnexpose(c *C) {
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

func (s *suite) TestClientServiceDestroy(c *C) {
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

func (s *suite) TestClientUnitResolved(c *C) {
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

var serviceDeployTests = []struct {
	about            string
	serviceName      string
	charmUrl         string
	numUnits         int
	expectedNumUnits int
	constraints      constraints.Value
}{{
	about:            "Normal deploy",
	serviceName:      "mywordpress",
	charmUrl:         "local:series/wordpress",
	expectedNumUnits: 1,
	constraints:      constraints.MustParse("mem=1G"),
}, {
	about:            "Two units",
	serviceName:      "mywordpress",
	charmUrl:         "local:series/wordpress",
	numUnits:         2,
	expectedNumUnits: 2,
	constraints:      constraints.MustParse("mem=4G"),
},
}

func (s *suite) TestClientServiceDeploy(c *C) {
	s.setUpScenario(c)

	for i, test := range serviceDeployTests {
		c.Logf("test %d; %s", i, test.about)
		parsedUrl := charm.MustParseURL(test.charmUrl)
		localRepo, err := charm.InferRepository(parsedUrl, coretesting.Charms.Path)
		c.Assert(err, IsNil)
		withRepo(localRepo, func() {
			_, err = s.State.Service(test.serviceName)
			c.Assert(state.IsNotFound(err), Equals, true)
			err = s.APIState.Client().ServiceDeploy(
				test.charmUrl, test.serviceName, test.numUnits, "", test.constraints,
			)
			c.Assert(err, IsNil)

			service, err := s.State.Service(test.serviceName)
			c.Assert(err, IsNil)
			defer removeServiceAndUnits(c, service)
			scons, err := service.Constraints()
			c.Assert(err, IsNil)
			c.Assert(scons, DeepEquals, test.constraints)

			units, err := service.AllUnits()
			c.Assert(err, IsNil)
			c.Assert(units, HasLen, test.expectedNumUnits)
			for _, unit := range units {
				mid, err := unit.AssignedMachineId()
				c.Assert(err, IsNil)
				machine, err := s.State.Machine(mid)
				c.Assert(err, IsNil)
				mcons, err := machine.Constraints()
				c.Assert(err, IsNil)
				c.Assert(mcons, DeepEquals, test.constraints)
			}
		})
	}
}

func withRepo(repo charm.Repository, f func()) {
	// Monkey-patch server repository.
	originalServerCharmStore := apiserver.CharmStore
	apiserver.CharmStore = repo
	defer func() {
		apiserver.CharmStore = originalServerCharmStore
	}()
	f()
}

func (s *suite) TestSuccessfulAddRelation(c *C) {
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

func (s *suite) TestSuccessfulDestroyRelation(c *C) {
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

func (s *suite) TestNoRelation(c *C) {
	s.setUpScenario(c)
	err := s.APIState.Client().DestroyRelation("wordpress", "mysql")
	c.Assert(err, ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}

func (s *suite) TestClientWatchAll(c *C) {
	// A very simple end-to-end test, because
	// all the logic is tested elsewhere.
	m, err := s.State.AddMachine("series", state.JobManageEnviron)
	c.Assert(err, IsNil)
	err = m.SetProvisioned("i-0", state.BootstrapNonce)
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
