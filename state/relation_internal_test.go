// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	rel := charm.Relation{
		Interface: "ifce",
		Name:      "group",
		Role:      charm.RolePeer,
		Scope:     charm.ScopeGlobal,
	}
	eps := []Endpoint{{
		ServiceName: "jeff",
		Relation:    rel,
	}, {
		ServiceName: "mike",
		Relation:    rel,
	}, {
		ServiceName: "mike",
		Relation:    rel,
	}}
	r := &Relation{nil, relationDoc{Endpoints: eps}}
	relatedEps, err := r.RelatedEndpoints("mike")
	c.Assert(err, IsNil)
	c.Assert(relatedEps, DeepEquals, eps)
}
