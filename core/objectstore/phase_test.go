// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/testing"
	tc "gopkg.in/check.v1"
)

type phaseSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&phaseSuite{})

func (s *phaseSuite) TestPhase(c *tc.C) {
	tests := []struct {
		value string
		err   string
	}{{
		value: "unknown",
	}, {
		value: "draining",
	}, {
		value: "error",
	}, {
		value: "completed",
	}, {
		value: "invalid",
		err:   `invalid phase "invalid"`,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		p, err := ParsePhase(test.value)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, tc.IsNil)
		c.Assert(p.String(), tc.Equals, test.value)
	}
}

func (s *phaseSuite) TestIsTerminal(c *tc.C) {
	tests := []struct {
		value    string
		expected bool
	}{{
		value:    "unknown",
		expected: false,
	}, {
		value:    "draining",
		expected: false,
	}, {
		value:    "error",
		expected: true,
	}, {
		value:    "completed",
		expected: true,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		p, err := ParsePhase(test.value)
		c.Assert(err, tc.IsNil)
		c.Assert(p.IsTerminal(), tc.Equals, test.expected)
	}
}

func (s *phaseSuite) TestIsValid(c *tc.C) {
	tests := []struct {
		value    Phase
		expected bool
	}{{
		value:    "unknown",
		expected: true,
	}, {
		value:    "draining",
		expected: true,
	}, {
		value:    "error",
		expected: true,
	}, {
		value:    "completed",
		expected: true,
	}, {
		value:    "invalid",
		expected: false,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		c.Assert(test.value.IsValid(), tc.Equals, test.expected)
	}
}

func (s *phaseSuite) TestIsDraining(c *tc.C) {
	tests := []struct {
		value    Phase
		expected bool
	}{{
		value:    "unknown",
		expected: false,
	}, {
		value:    "draining",
		expected: true,
	}, {
		value:    "error",
		expected: false,
	}, {
		value:    "completed",
		expected: false,
	}, {
		value:    "invalid",
		expected: false,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.value)

		c.Assert(test.value.IsDraining(), tc.Equals, test.expected)
	}
}

func (s *phaseSuite) TestTransitionTo(c *tc.C) {
	tests := []struct {
		from     string
		to       string
		expected string
		err      string
	}{{
		from:     "unknown",
		to:       "draining",
		expected: "draining",
	}, {
		from:     "draining",
		to:       "error",
		expected: "error",
	}, {
		from:     "draining",
		to:       "completed",
		expected: "completed",
	}, {
		from: "error",
		to:   "unknown",
		err:  `invalid transition from "error" to "unknown"`,
	}, {
		from: "completed",
		to:   "unknown",
		err:  `invalid transition from "completed" to "unknown"`,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s -> %s", i, test.from, test.to)

		pFrom, err := ParsePhase(test.from)
		c.Assert(err, tc.IsNil)
		pTo, err := ParsePhase(test.to)
		c.Assert(err, tc.IsNil)

		newPhase, err := pFrom.TransitionTo(pTo)
		if test.err != "" {
			c.Assert(err, tc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, tc.IsNil)
		c.Assert(newPhase.String(), tc.Equals, test.expected)
	}
}
