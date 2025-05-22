// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringcompare_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/stringcompare"
)

type StringComparatorSuite struct{}

func TestStringComparatorSuite(t *stdtesting.T) {
	tc.Run(t, &StringComparatorSuite{})
}

func (*StringComparatorSuite) TestLevenshteinDistance(c *tc.C) {
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

	for _, testCase := range testCases {
		c.Check(stringcompare.LevenshteinDistance(testCase.input1, testCase.input2), tc.Equals, testCase.expectedResult,
			tc.Commentf("Description: %s | Inputs: '%s', '%s'", testCase.desc, testCase.input1, testCase.input2),
		)
	}
}
