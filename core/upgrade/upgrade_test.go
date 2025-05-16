// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type upgradeSuite struct {
	testhelpers.IsolationSuite
}

func TestUpgradeSuite(t *stdtesting.T) { tc.Run(t, &upgradeSuite{}) }
func (s *upgradeSuite) TestParseState(c *tc.C) {
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
	}, {
		str: "error",
		st:  Error,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.str)

		st, err := ParseState(test.str)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
			continue
		}
		c.Check(err, tc.IsNil)
		c.Check(st, tc.Equals, test.st)
	}
}

func (s *upgradeSuite) TestIsTerminal(c *tc.C) {
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
	}, {
		st:       Error,
		terminal: true,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.st)

		terminal := test.st.IsTerminal()
		c.Check(terminal, tc.Equals, test.terminal)
	}
}

func (s *upgradeSuite) TestTransitionTo(c *tc.C) {
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
				c.Check(err, tc.Equals, ErrAlreadyAtState)
				continue
			}
			if st == test.target && !test.st.IsTerminal() {
				c.Check(err, tc.IsNil)
				continue
			}
			c.Check(err, tc.ErrorIs, ErrUnableToTransition)
		}
	}
}

func (s *upgradeSuite) TestTransitionToError(c *tc.C) {
	// Brute force test all possible transitions.
	tests := []struct {
		st  State
		err error
	}{{
		st: Created,
	}, {
		st: Started,
	}, {
		st: DBCompleted,
	}, {
		st: StepsCompleted,
	}, {
		st:  Error,
		err: ErrAlreadyAtState,
	}}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.st)

		err := test.st.TransitionTo(Error)
		if test.err != nil {
			c.Check(err, tc.ErrorIs, test.err)
			continue
		}
		c.Check(err, tc.IsNil)
	}
}
