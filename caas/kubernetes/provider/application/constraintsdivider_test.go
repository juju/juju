// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	gc "gopkg.in/check.v1"
)

type constraintsDividerSuite struct{}

var _ = gc.Suite(&constraintsDividerSuite{})

func (s *constraintsDividerSuite) TestDivideAndSpreadContainerResource(c *gc.C) {
	type input struct {
		totalResource uint64
		numContainers int
	}

	tests := []struct {
		desc           string
		in             input
		expectedResult []uint64
	}{
		{
			desc:           "Evenly divisible",
			in:             input{totalResource: 10, numContainers: 2},
			expectedResult: []uint64{5, 5},
		},
		{
			desc:           "Remainder distributed to front",
			in:             input{totalResource: 10, numContainers: 3},
			expectedResult: []uint64{4, 3, 3},
		},
		{
			desc:           "Remainder distributed to multiple fronts",
			in:             input{totalResource: 10, numContainers: 4},
			expectedResult: []uint64{3, 3, 2, 2},
		},
		{
			desc:           "More containers than total",
			in:             input{totalResource: 3, numContainers: 5},
			expectedResult: []uint64{1, 1, 1, 0, 0},
		},
		{
			desc:           "Zero resource",
			in:             input{totalResource: 0, numContainers: 3},
			expectedResult: []uint64{0, 0, 0},
		},
		{
			desc:           "Single container",
			in:             input{totalResource: 10, numContainers: 1},
			expectedResult: []uint64{10},
		},
		{
			desc:           "Total resource value equals number of containers",
			in:             input{totalResource: 10, numContainers: 10},
			expectedResult: []uint64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		},
		{
			desc:           "Zero containers (edge case)",
			in:             input{totalResource: 10, numContainers: 0},
			expectedResult: nil,
		},
	}

	for _, tc := range tests {
		obtained := divideAndSpreadContainerResource(tc.in.totalResource, tc.in.numContainers)
		c.Assert(obtained, gc.DeepEquals, tc.expectedResult)

		// ensure sum matches total
		if tc.in.numContainers > 0 && obtained != nil {
			var sum uint64
			for _, v := range obtained {
				sum += v
			}
			c.Assert(sum, gc.Equals, tc.in.totalResource)
		}
	}
}
