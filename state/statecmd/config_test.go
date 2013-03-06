package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

type ConfigSuite struct {
	testing.JujuConnSuite
}

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = Suite(&ConfigSuite{})

func serviceSet(p params.ServiceSet) func(st *state.State) error {
	return func(st *state.State) error {
		return statecmd.ServiceSet(st, p)
	}
}

func serviceSetYAML(p params.ServiceSetYAML) func(st *state.State) error {
	return func(st *state.State) error {
		return statecmd.ServiceSetYAML(st, p)
	}
}

var serviceSetTests = []struct {
	about   string
	initial map[string]interface{}
	set     func(st *state.State) error
	expect  map[string]interface{} // resulting configuration of the dummy service.
	err     string                 // error regex
}{
	{
		about: "unknown service name",
		set: serviceSet(params.ServiceSet{
			ServiceName: "unknown-service",
			Options: map[string]string{
				"foo": "bar",
			},
		}),
		err: `service "unknown-service" not found`,
	}, {
		about: "no config or options",
		set:   serviceSet(params.ServiceSet{}),
		err:   "no options to set",
	}, {
		about: "unknown option",
		set: serviceSet(params.ServiceSet{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"foo": "bar",
			},
		}),
		err: `Unknown configuration option: "foo"`,
	}, {
		about: "set outlook",
		set: serviceSet(params.ServiceSet{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"outlook": "positive",
			},
		}),
		expect: map[string]interface{}{
			"outlook": "positive",
		},
	}, {
		about: "unset outlook and set title",
		initial: map[string]interface{}{
			"outlook": "positive",
		},
		set: serviceSet(params.ServiceSet{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"outlook": "",
				"title":   "sir",
			},
		}),
		expect: map[string]interface{}{
			"title": "sir",
		},
	}, {
		about: "set a default value",
		initial: map[string]interface{}{
			"title": "sir",
		},
		set: serviceSet(params.ServiceSet{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"username": "admin001",
			},
		}),
		expect: map[string]interface{}{
			"username": "admin001",
			"title":    "sir",
		},
	}, {
		about: "unset a default value, set a different default",
		initial: map[string]interface{}{
			"username": "admin001",
			"title":    "sir",
		},
		set: serviceSet(params.ServiceSet{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"username": "",
				"title":    "My Title",
			},
		}),
		expect: map[string]interface{}{
			"title": "My Title",
		},
	}, {
		about: "bad configuration",
		set: serviceSetYAML(params.ServiceSetYAML{
			Config: "345",
		}),
		err: "no options to set",
	}, {
		about: "config with no options",
		set: serviceSetYAML(params.ServiceSetYAML{
			Config: "{}",
		}),
		err: "no options to set",
	}, {
		about: "set some attributes",
		initial: map[string]interface{}{
			"title": "sir",
		},
		set: serviceSetYAML(params.ServiceSetYAML{
			ServiceName: "dummy-service",
			Config:      "skill-level: 9000\nusername: admin001\n\n",
		}),
		expect: map[string]interface{}{
			"title":       "sir",
			"username":    "admin001",
			"skill-level": int64(9000), // yaml int types are int64
		},
	}, {
		about: "remove an attribute by setting to empty string",
		initial: map[string]interface{}{
			"title":    "sir",
			"username": "foo",
		},
		set: serviceSetYAML(params.ServiceSetYAML{
			ServiceName: "dummy-service",
			Config:      "title: ''\n",
		}),
		expect: map[string]interface{}{
			"username": "foo",
		},
	},
}

func (s *ConfigSuite) TestServiceSet(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	for i, t := range serviceSetTests {
		c.Logf("test %d. %s", i, t.about)
		svc, err := s.State.AddService("dummy-service", sch)
		c.Assert(err, IsNil)
		if t.initial != nil {
			cfg, err := svc.Config()
			c.Assert(err, IsNil)
			cfg.Update(t.initial)
			_, err = cfg.Write()
			c.Assert(err, IsNil)
		}
		err = t.set(s.State)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			cfg, err := svc.Config()
			c.Assert(err, IsNil)
			c.Assert(cfg.Map(), DeepEquals, t.expect)
		}
		err = svc.Destroy()
		c.Assert(err, IsNil)
	}
}

var getTests = []struct {
	about  string
	params params.ServiceGet // parameters to ServiceGet call.
	expect params.ServiceGetResults
	err    string
}{
	{
		about: "unknown service name",
		params: params.ServiceGet{
			ServiceName: "unknown-service",
		},
		err: `service "unknown-service" not found`,
	},
	{
		about: "deployed service",
		params: params.ServiceGet{
			ServiceName: "dummy-service",
		},
		expect: params.ServiceGetResults{
			Service: "dummy-service",
			Charm:   "dummy",
			Settings: map[string]interface{}{
				"outlook": map[string]interface{}{
					"description": "No default outlook.",
					"type":        "string",
					"value":       nil,
				},
				"username": map[string]interface{}{
					"description": "The name of the initial account (given admin permissions).",
					"type":        "string",
					"value":       nil,
				},
				"skill-level": map[string]interface{}{
					"description": "A number indicating skill.",
					"type":        "int",
					"value":       nil,
				},
				"title": map[string]interface{}{
					"description": "A descriptive title used for the service.",
					"type":        "string",
					"value":       nil,
				},
			},
		},
	},
}

func (s *ConfigSuite) TestServiceGet(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, IsNil)
	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		results, err := statecmd.ServiceGet(s.State, t.params)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			c.Assert(results, DeepEquals, t.expect)
		}
	}
}
