// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type StateSuite struct {
	testhelpers.IsolationSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &StateSuite{})
}

func (s *StateSuite) TestParseStateKnown(c *tc.C) {
	recognized := map[string]State{
		"potential": StatePotential,
		"available": StateAvailable,
	}
	for value, expected := range recognized {
		state, err := ParseState(value)

		c.Check(err, tc.ErrorIsNil)
		c.Check(state, tc.Equals, expected)
	}
}

func (s *StateSuite) TestParseStateUnknown(c *tc.C) {
	_, err := ParseState("<invalid>")

	c.Check(err, tc.ErrorMatches, `.*state "<invalid>" invalid.*`)
}

func (s *StateSuite) TestValidateKnown(c *tc.C) {
	recognized := []State{
		StatePotential,
		StateAvailable,
	}
	for _, state := range recognized {
		err := state.Validate()

		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *StateSuite) TestValidateUnknown(c *tc.C) {
	var state State
	err := state.Validate()

	c.Check(err, tc.ErrorMatches, `.*state "" invalid.*`)
}
