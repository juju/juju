// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type StateSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&StateSuite{})

func (StateSuite) TestParseStateKnown(c *tc.C) {
	recognized := map[string]State{
		"potential": StatePotential,
		"available": StateAvailable,
	}
	for value, expected := range recognized {
		state, err := ParseState(value)

		c.Check(err, jc.ErrorIsNil)
		c.Check(state, tc.Equals, expected)
	}
}

func (StateSuite) TestParseStateUnknown(c *tc.C) {
	_, err := ParseState("<invalid>")

	c.Check(err, tc.ErrorMatches, `.*state "<invalid>" invalid.*`)
}

func (StateSuite) TestValidateKnown(c *tc.C) {
	recognized := []State{
		StatePotential,
		StateAvailable,
	}
	for _, state := range recognized {
		err := state.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (StateSuite) TestValidateUnknown(c *tc.C) {
	var state State
	err := state.Validate()

	c.Check(err, tc.ErrorMatches, `.*state "" invalid.*`)
}
