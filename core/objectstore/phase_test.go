// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type phaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&phaseSuite{})

func (s *phaseSuite) TestPhase(c *gc.C) {
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
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(p.String(), gc.Equals, test.value)
	}
}

func (s *phaseSuite) TestIsTerminal(c *gc.C) {
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
		c.Assert(err, gc.IsNil)
		c.Assert(p.IsTerminal(), gc.Equals, test.expected)
	}
}

func (s *phaseSuite) TestTransitionTo(c *gc.C) {
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
		c.Assert(err, gc.IsNil)
		pTo, err := ParsePhase(test.to)
		c.Assert(err, gc.IsNil)

		newPhase, err := pFrom.TransitionTo(pTo)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(newPhase.String(), gc.Equals, test.expected)
	}
}
