// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type querySuite struct{}

var _ = gc.Suite(&querySuite{})

func (s *querySuite) TestQuery(c *gc.C) {
	tests := []struct {
		Source string
		Result Query
	}{
		{
			Source: "life=alive",
			Result: Query{
				Arguments: map[string][]string{
					"life": {"alive"},
				},
			},
		},
		{
			Source: "life=alive,dying",
			Result: Query{
				Arguments: map[string][]string{
					"life": {"alive", "dying"},
				},
			},
		},
		{
			Source: "life=alive; life=dying",
			Result: Query{
				Arguments: map[string][]string{
					"life": {"alive", "dying"},
				},
			},
		},
		{
			Source: "life=alive,dying; status=active",
			Result: Query{
				Arguments: map[string][]string{
					"life":   {"alive", "dying"},
					"status": {"active"},
				},
			},
		},
		{
			Source: `life=alive dying; status=active`,
			Result: Query{
				Arguments: map[string][]string{
					"life":   {"alive", "dying"},
					"status": {"active"},
				},
			},
		},
		{
			Source: `life="alive dying"; status=active`,
			Result: Query{
				Arguments: map[string][]string{
					"life":   {"alive dying"},
					"status": {"active"},
				},
			},
		},
		{
			Source: `life=alive,dying status=active`,
			Result: Query{
				Arguments: map[string][]string{
					"life":   {"alive", "dying"},
					"status": {"active"},
				},
			},
		},
	}
	for _, test := range tests {
		query, err := Parse(test.Source)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(query, gc.DeepEquals, test.Result)
	}
}
