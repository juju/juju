package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju/testing"
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

var getTests = []struct {
	about  string
	params params.ServiceGet
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
			Config: map[string]interface{}{
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
			Constraints: constraints.MustParse("mem=2G cpu-power=400"),
		},
	},
}

func (s *ConfigSuite) TestServiceGet(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, IsNil)
	err = svc.SetConstraints(constraints.MustParse("mem=2G cpu-power=400"))
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
