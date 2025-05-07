// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import "github.com/juju/tc"

type APIHelperSuite struct {
}

var _ = tc.Suite(&APIHelperSuite{})

var moveToFrontTests = []struct {
	item   string
	items  []string
	expect []string
}{{
	item:   "x",
	items:  []string{"y", "x"},
	expect: []string{"x", "y"},
}, {
	item:   "z",
	items:  []string{"y", "x"},
	expect: []string{"y", "x"},
}, {
	item:   "y",
	items:  []string{"y", "x"},
	expect: []string{"y", "x"},
}, {
	item:   "x",
	items:  []string{"y", "x", "z"},
	expect: []string{"x", "y", "z"},
}, {
	item:   "d",
	items:  []string{"a", "b", "c", "d", "e", "f"},
	expect: []string{"d", "a", "b", "c", "e", "f"},
}}

func (s *APIHelperSuite) TestMoveToFront(c *tc.C) {
	for i, test := range moveToFrontTests {
		c.Logf("test %d: moveToFront %q %v", i, test.item, test.items)
		moveToFront(test.item, test.items)
		c.Check(test.items, tc.DeepEquals, test.expect)
	}
}

var addrsChangedTests = []struct {
	description string
	source      []string
	target      []string
	changed     bool
	setDiff     bool
}{{
	description: "first longer",
	source:      []string{"a", "b"},
	target:      []string{"a"},
	changed:     true,
	setDiff:     true,
}, {
	description: "second longer",
	source:      []string{"a"},
	target:      []string{"a", "b"},
	changed:     true,
	setDiff:     true,
}, {
	description: "identical",
	source:      []string{"a", "b"},
	target:      []string{"a", "b"},
	changed:     false,
	setDiff:     false,
}, {
	description: "reordered",
	source:      []string{"b", "a"},
	target:      []string{"a", "b"},
	changed:     true,
	setDiff:     false,
}, {
	description: "repeated",
	source:      []string{"a", "b", "c"},
	target:      []string{"a", "b", "b"},
	changed:     true,
	setDiff:     true,
}, {
	description: "repeated reversed",
	source:      []string{"a", "b", "b"},
	target:      []string{"a", "b", "c"},
	changed:     true,
	setDiff:     true,
}}

func (s *APIHelperSuite) TestAddrsChanged(c *tc.C) {
	for i, test := range addrsChangedTests {
		c.Logf("test %d: addrsChanged %v %v", i, test.description)
		anyChange, differentSet := addrsChanged(test.source, test.target)
		c.Check(anyChange, tc.Equals, test.changed,
			tc.Commentf("%v vs %v declared that %t but expected %t",
				test.source, test.target, anyChange, test.changed))
		c.Check(differentSet, tc.Equals, test.setDiff,
			tc.Commentf("%v vs %v declared that %t but expected %t",
				test.source, test.target, differentSet, test.setDiff))
	}
}
