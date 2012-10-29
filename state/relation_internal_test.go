package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
)

type RelationSuite struct{}

var _ = Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *C) {
	r := &Relation{nil, relationDoc{Endpoints: []Endpoint{
		Endpoint{"jeff", "ifce", "group", RolePeer, charm.ScopeGlobal},
		Endpoint{"mike", "ifce", "group", RolePeer, charm.ScopeGlobal},
		Endpoint{"bill", "ifce", "group", RolePeer, charm.ScopeGlobal},
	}}}
	eps, err := r.RelatedEndpoints("mike")
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []Endpoint{
		Endpoint{"jeff", "ifce", "group", RolePeer, charm.ScopeGlobal},
		Endpoint{"mike", "ifce", "group", RolePeer, charm.ScopeGlobal},
		Endpoint{"bill", "ifce", "group", RolePeer, charm.ScopeGlobal},
	})
}
