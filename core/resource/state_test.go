// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type StateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StateSuite{})

func (StateSuite) TestParseStateKnown(c *gc.C) {
	recognized := map[string]State{
		"potential": StatePotential,
		"available": StateAvailable,
	}
	for value, expected := range recognized {
		state, err := ParseState(value)

		c.Check(err, jc.ErrorIsNil)
		c.Check(state, gc.Equals, expected)
	}
}

func (StateSuite) TestParseStateUnknown(c *gc.C) {
	_, err := ParseState("<invalid>")

	c.Check(err, gc.ErrorMatches, `.*state "<invalid>" invalid.*`)
}

func (StateSuite) TestValidateKnown(c *gc.C) {
	recognized := []State{
		StatePotential,
		StateAvailable,
	}
	for _, state := range recognized {
		err := state.Validate()

		c.Check(err, jc.ErrorIsNil)
	}
}

func (StateSuite) TestValidateUnknown(c *gc.C) {
	var state State
	err := state.Validate()

	c.Check(err, gc.ErrorMatches, `.*state "" invalid.*`)
}
