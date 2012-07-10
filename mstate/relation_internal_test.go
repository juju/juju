package mstate

import (
	. "launchpad.net/gocheck"
)

type RelationSuite struct{}

var _ = Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *C) {
	r := &Relation{nil, 0, []RelationEndpoint{
		RelationEndpoint{"jeff", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"mike", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"bill", "ifce", "group", RolePeer, ScopeGlobal},
	}}
	eps, err := r.RelatedEndpoints("mike")
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []RelationEndpoint{
		RelationEndpoint{"jeff", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"mike", "ifce", "group", RolePeer, ScopeGlobal},
		RelationEndpoint{"bill", "ifce", "group", RolePeer, ScopeGlobal},
	})
}
