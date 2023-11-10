// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facades

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type FacadeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestBestVersion(c *gc.C) {
	tests := []struct {
		versions []int
		desired  []int
		expected int
	}{{
		versions: []int{1, 2, 3},
		desired:  []int{1},
		expected: 1,
	}, {
		versions: []int{1, 2, 3},
		desired:  []int{1, 2},
		expected: 2,
	}, {
		versions: []int{1, 2, 3},
		desired:  []int{1, 2, 3},
		expected: 3,
	}, {
		versions: []int{},
		desired:  []int{0, 1, 2},
		expected: 0,
	}}
	for i, test := range tests {
		c.Logf("test %d", i)
		c.Check(BestVersion(test.desired, test.versions), gc.Equals, test.expected)
	}
}

func (s *FacadeSuite) TestCompleteIntersection(c *gc.C) {
	tests := []struct {
		src      FacadeVersions
		dst      FacadeVersions
		expected bool
	}{{
		src: FacadeVersions{
			"foo": []int{1, 2, 3},
		},
		dst: FacadeVersions{
			"foo": []int{1, 2, 3},
		},
		expected: true,
	}, {
		src: FacadeVersions{
			"bar": []int{1, 2, 3},
		},
		dst: FacadeVersions{
			"foo": []int{1, 2, 3},
		},
		expected: false,
	}, {
		src: FacadeVersions{
			"foo": []int{3, 4, 5},
		},
		dst: FacadeVersions{
			"foo": []int{1, 2, 3},
		},
		expected: true,
	}, {
		src: FacadeVersions{
			"foo": []int{4, 5},
		},
		dst: FacadeVersions{
			"foo": []int{1, 2, 3},
		},
		expected: false,
	}, {
		src: FacadeVersions{
			"foo": []int{2, 3, 4},
		},
		dst: FacadeVersions{
			"foo": []int{1},
		},
		expected: false,
	}, {
		src: FacadeVersions{
			"foo": []int{1, 2, 3},
			"bar": []int{3},
		},
		dst: FacadeVersions{
			"foo": []int{1, 2, 3},
			"bar": []int{1, 3},
		},
		expected: true,
	}}
	for i, test := range tests {
		c.Logf("test %d", i)
		c.Check(CompleteIntersection(test.src, test.dst), gc.Equals, test.expected)
	}
}
