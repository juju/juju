// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/unitless"
	unitlessstate "github.com/juju/juju/domain/unitless/state"
)

type stateSuite struct{}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetApplicationScriptlet(c *tc.C) {
	scriptlet, err := unitlessstate.NewState(nil).GetApplicationScriptlet(
		c.Context(), "application-uuid",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(scriptlet, tc.DeepEquals, unitless.Scriptlet{})
}

func (s *stateSuite) TestGetScriptletEvent(c *tc.C) {
	event, err := unitlessstate.NewState(nil).GetScriptletEvent(
		c.Context(), "application-uuid", "config-changed",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(event, tc.DeepEquals, unitless.Event{})
}
