// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type sprintfSuite struct {
	coretesting.BaseSuite
}

func TestSprintfSuite(t *testing.T) {
	tc.Run(t, &sprintfSuite{})
}

func (s *sprintfSuite) TestVerbs(c *tc.C) {
	tests := []struct {
		in   string
		want string
	}{{
		in:   "SELECT %s FROM t",
		want: "SELECT t FROM t",
	}, {
		in:   "NEW.%[1]s AND x = 1",
		want: "NEW.t AND x = 1",
	}, {
		in:   "VALUES (%d, %s)",
		want: "VALUES (1, t)",
	}}
	for i, test := range tests {
		c.Logf("case %d: %s", i, test.in)
		got := replaceSprintfVerbs(literalEval(test.in))
		c.Check(got.Content, tc.Equals, test.want)
	}
}
