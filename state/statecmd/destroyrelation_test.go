package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
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
		about:     "destroy an already destroyed relation",
		endpoints: []string{"wordpress", "mysql"},
		err:       `relation "wordpress:db mysql:server" not found`,
	},
}

func (s *DestroyRelationSuite) TestDestroyRelation(c *C) {
	// Create some services.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")

	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")

	_, err = s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)

	// Add a relation between wordpress and mysql.
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
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
			for _, svc := range []*state.Service{wordpress, mysql} {
				//svc.Refresh()
				rels, err := svc.Relations()
				c.Assert(err, IsNil)
				c.Assert(rels, HasLen, 0)
			}
		}
	}
}
