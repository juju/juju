// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringcompare_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/stringcompare"
)

type StringComparatorSuite struct{}

var _ = gc.Suite(&StringComparatorSuite{})

func (*StringComparatorSuite) TestLevenshteinDistance(c *gc.C) {
	testCases := []struct {
		input1, input2 string
		expectedResult int
		desc           string
	}{
		{"", "", 0, "both strings are empty"},
		{"", "abc", 3, "first string is empty"},
		{"abc", "", 3, "second string is empty"},
		{"abc", "abc", 0, "both strings are identical"},
		{"abc", "def", 3, "completely different strings"},
		{"fly", "cry", 2, "simple case with substitutions"},
		{"playing", "sharing", 3, "normal case with substitutions and insertions"},
		{"distance", "editing", 5, "more complex strings with multiple edits"},
		{"aaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbb", 24, "long strings"},
		{"à", "à", 0, "similar unicode strings"},
	}

	for _, tc := range testCases {
		c.Check(stringcompare.LevenshteinDistance(tc.input1, tc.input2), gc.Equals, tc.expectedResult,
			gc.Commentf("Description: %s | Inputs: '%s', '%s'", tc.desc, tc.input1, tc.input2),
		)
	}
}
