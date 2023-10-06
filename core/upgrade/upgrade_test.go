// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type upgradeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&upgradeSuite{})

func (s *upgradeSuite) TestParseState(c *gc.C) {
	tests := []struct {
		str string
		st  State
		err string
	}{{
		str: "",
		st:  0,
		err: `unknown state ""`,
	}, {
		str: "created",
		st:  Created,
	}, {
		str: "started",
		st:  Started,
	}, {
		str: "db-completed",
		st:  DBCompleted,
	}, {
		str: "steps-completed",
		st:  StepsCompleted,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.str)

		st, err := ParseState(test.str)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Check(err, gc.IsNil)
		c.Check(st, gc.Equals, test.st)
	}
}

func (s *upgradeSuite) TestIsTerminal(c *gc.C) {
	tests := []struct {
		st       State
		terminal bool
	}{{
		st: Created,
	}, {
		st: Started,
	}, {
		st: DBCompleted,
	}, {
		st:       StepsCompleted,
		terminal: true,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.st)

		terminal := test.st.IsTerminal()
		c.Check(terminal, gc.Equals, test.terminal)
	}
}

func (s *upgradeSuite) TestTransitionTo(c *gc.C) {
	// Brute force test all possible transitions.
	states := []State{Created, Started, DBCompleted, StepsCompleted}
	tests := []struct {
		st     State
		target State
	}{{
		st:     Created,
		target: Started,
	}, {
		st:     Started,
		target: DBCompleted,
	}, {
		st:     DBCompleted,
		target: StepsCompleted,
	}, {
		st: StepsCompleted,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.st)

		for _, st := range states {
			err := test.st.TransitionTo(st)

			if test.st == st {
				c.Check(err, gc.Equals, ErrAlreadyAtState)
				continue
			}
			if st == test.target && !test.st.IsTerminal() {
				c.Check(err, gc.IsNil)
				continue
			}
			c.Check(err, gc.ErrorMatches, `cannot transition from .* to .*`)
		}
	}
}
