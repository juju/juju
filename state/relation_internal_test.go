// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
)

type RelationSuite struct{}

var _ = gc.Suite(&RelationSuite{})

// TestRelatedEndpoints verifies the behaviour of RelatedEndpoints in
// multi-endpoint peer relations, which are currently not constructable
// by normal means.
func (s *RelationSuite) TestRelatedEndpoints(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relatedEps, gc.DeepEquals, eps)
}
