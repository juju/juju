package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
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

var serviceSetTests = []struct {
	about  string
	initial map[string]interface{}
	params statecmd.ServiceSetParams // parameters to ServiceSet call.
	expect map[string]interface{}    // resulting configuration of the dummy service.
	err    string                    // error regex
}{
	{
		about: "unknown service name",
		params: statecmd.ServiceSetParams{
			ServiceName: "unknown-service",
			Options: map[string]string{
				"foo": "bar",
			},
		},
		err: `service "unknown-service" not found`,
	}, {
		about:  "no config or options",
		params: statecmd.ServiceSetParams{},
		err:    "no options to set",
	}, {
		about: "unknown option",
		params: statecmd.ServiceSetParams{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"foo": "bar",
			},
		},
		err: `Unknown configuration option: "foo"`,
	}, {
		about: "set outlook",
		params: statecmd.ServiceSetParams{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"outlook": "positive",
			},
		},
		expect: map[string]interface{}{
			"outlook": "positive",
		},
	}, {
		about: "unset outlook and set title",
		initial: map[string]interface{}{
			"outlook": "positive",
		},
		params: statecmd.ServiceSetParams{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"outlook": "",
				"title":   "sir",
			},
		},
		expect: map[string]interface{}{
			"title": "sir",
		},
	}, {
		about: "set a default value",
		initial: map[string]interface{}{
			"title": "sir",
		},
		params: statecmd.ServiceSetParams{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"username": "admin001",
			},
		},
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
		params: statecmd.ServiceSetParams{
			ServiceName: "dummy-service",
			Options: map[string]string{
				"username": "",
				"title":    "My Title",
			},
		},
		expect: map[string]interface{}{
			"title": "My Title",
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
		err = statecmd.ServiceSet(s.State, t.params)
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
//
//// TODO(rog) make these tests independent of one another.
//var setTests = []struct {
//	about  string
//	params statecmd.ServiceSetParams // parameters to ServiceSet call.
//	expect map[string]interface{}    // resulting configuration of the dummy service.
//	err    string                    // error regex
//}{ {
//		about: "bad configuration",
//		params: statecmd.ServiceSetParams{
//			Config: "345",
//		},
//		err: "no options to set",
//	}, {
//		about: "config with no options",
//		params: statecmd.ServiceSetParams{
//			Config: "{}",
//		},
//		err: "no options to set",
//	}, {
//		about: "yaml config",
//		initial: map[string]interface{
//			"title":    "sir",
//		},
//		params: statecmd.ServiceSetParams{
//			ServiceName: "dummy-service",
//			Config:      "skill-level: 9000\nusername: admin001\n\n",
//		},
//		expect: map[string]interface{}{
//			"title":       "My Title",
//			"username":    "admin001",
//			"skill-level": int64(9000), // yaml int types are int64
//		},
//	},
//}
//
//
//func (s *ConfigSuite) TestServiceSetYAML(c *C) {
//	sch := s.AddTestingCharm(c, "dummy")
//	svc, err := s.State.AddService("dummy-service", sch)
//	c.Assert(err, IsNil)
//	for i, t := range setTests {
//		c.Logf("test %d. %s", i, t.about)
//		err = statecmd.ServiceSet(s.State, t.params)
//		if t.err != "" {
//			c.Assert(err, ErrorMatches, t.err)
//		} else {
//			c.Assert(err, IsNil)
//			cfg, err := svc.Config()
//			c.Assert(err, IsNil)
//			c.Assert(cfg.Map(), DeepEquals, t.expect)
//		}
//	}
//}

var getTests = []struct {
	about  string
	params statecmd.ServiceGetParams // parameters to ServiceGet call.
	expect *statecmd.ServiceGetResults
	err    string
}{
	{
		about: "unknown service name",
		params: statecmd.ServiceGetParams{
			ServiceName: "unknown-service",
		},
		err: `service "unknown-service" not found`,
	},
	{
		about: "deployed service",
		params: statecmd.ServiceGetParams{
			ServiceName: "dummy-service",
		},
		expect: &statecmd.ServiceGetResults{
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
