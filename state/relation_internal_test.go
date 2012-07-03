package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type RelationSuite struct{}

var _ = Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *C) {
	r := &Relation{nil, "", []RelationEndpoint{
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

type UnitScopePathSuite struct {
	testing.ZkConnSuite
}

var _ = Suite(&UnitScopePathSuite{})

func (s *UnitScopePathSuite) TestPaths(c *C) {
	usp := unitScopePath("/path/to/scope")
	c.Assert(usp.settingsPath("u-551"), Equals, "/path/to/scope/settings/u-551")
	c.Assert(usp.presencePath(RolePeer, "u-551"), Equals, "/path/to/scope/peer/u-551")
}

func (s *UnitScopePathSuite) TestPrepareJoin(c *C) {
	usp := unitScopePath("/scope")
	err := usp.prepareJoin(s.ZkConn, RoleRequirer)
	c.Assert(err, IsNil)
	stat, err := s.ZkConn.Exists("/scope/requirer")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
	stat, err = s.ZkConn.Exists("/scope/settings")
	c.Assert(err, IsNil)
	c.Assert(stat, NotNil)
}
