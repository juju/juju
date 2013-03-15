package statecmd_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type DestroyRelationSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&DestroyRelationSuite{})

var destroyRelationTests = []struct {
	about     string
	endpoints []string
	err       string
}{
	{
		about:     "non-existent relation",
		endpoints: []string{"riak", "wordpress"},
		err:       "no relations found",
	},
	{
		about:     "successfully destroy a relation",
		endpoints: []string{"wordpress", "mysql"},
	},
	{
		about:     "successfully destroy a relation, swapping the order.",
		endpoints: []string{"logging", "wordpress"},
	},
	{
		about:     "destroy an already destroyed relation",
		endpoints: []string{"wordpress", "mysql"},
		err:       `relation "wordpress:db mysql:server" not found`,
	},
}

func (s *DestroyRelationSuite) TestDestroyRelation(c *C) {
	// Create some services.
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	// And a relation between wordpress and logging.
	eps, err = s.State.InferEndpoints([]string{"wordpress", "logging"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	for i, t := range destroyRelationTests {
		c.Logf("test %d. %s", i, t.about)
		err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
			Endpoint0: t.endpoints[0],
			Endpoint1: t.endpoints[1],
		})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			c.Assert(err, IsNil)
			// Show that the relation was removed.
			eps, err := s.State.InferEndpoints([]string{t.endpoints[0], t.endpoints[1]})
			_, err = s.State.EndpointsRelation(eps...)
			expected := fmt.Sprintf("relation \"%s:.* %s:.*\" not found",
				t.endpoints[0], t.endpoints[1])
			c.Assert(err, ErrorMatches, expected)
		}
	}
}
