// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type predicatesSuite struct{}

var _ = gc.Suite(&predicatesSuite{})

func (s *predicatesSuite) TestBoolean(c *gc.C) {
	result := BoolPredicate(true)(nil)
	c.Assert(result, jc.IsTrue)

	result = BoolPredicate(false)(nil)
	c.Assert(result, jc.IsFalse)
}

type interpreterSuite struct{}

var _ = gc.Suite(&interpreterSuite{})

func (s *interpreterSuite) TestInterpreter(c *gc.C) {
	tests := []struct {
		Query  Query
		Entity params.EntityInfo
		Result bool
	}{
		{
			Query: Query{
				Arguments: map[string][]string{
					"life":   {"alive", "dying"},
					"status": {"active"},
				},
			},
			Entity: &params.ApplicationInfo{
				Life: life.Alive,
				Status: params.StatusInfo{
					Current: "active",
				},
			},
			Result: true,
		},
		{
			Query: Query{
				Arguments: map[string][]string{
					"life":   {"dead"},
					"status": {"active"},
				},
			},
			Entity: &params.ApplicationInfo{
				Life: life.Alive,
				Status: params.StatusInfo{
					Current: "active",
				},
			},
			Result: false,
		},
		{
			Query: Query{
				Arguments: map[string][]string{},
			},
			Entity: &params.ApplicationInfo{
				Life: life.Alive,
				Status: params.StatusInfo{
					Current: "active",
				},
			},
			Result: true,
		},
	}
	for _, test := range tests {
		predicate, err := PredicateInterpreter(test.Query)
		c.Assert(err, jc.ErrorIsNil)
		result := predicate(test.Entity)
		c.Assert(result, gc.Equals, test.Result)
	}
}
